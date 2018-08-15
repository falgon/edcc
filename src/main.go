package main

import (
	"./cs3"
	"./csns"
	"./cssh"
	"./instance"
	"./net"
	"./textemplate"
	"./utils"
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/fatih/color"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var (
	ArgRegion               = flag.String("region", "ap-northeast-1", "region setting")
	ArgCidrBlock            = flag.String("cidr", "10.0.0.0/16", "cidr block")
	ArgSubnetBlock          = flag.String("subnet", "10.0.0.0/24", "subnet block")
	ArgTopicName            = flag.String("topic-name", "", "topic name")
	ArgNotificationEndpoint = flag.String("notification-endpoint", "", "notification endpoint")
	ArgKeyName              = flag.String("key-name", "", "key name")
	ArgKeyPath              = flag.String("key-path", "./"+*ArgKeyName, "key path")
	ArgBucketName           = flag.String("bucket-name", "", "bucket name")
	ArgSetUpScript          = flag.String("setup-script", "", "setup script path")
	ArgInputScript          = flag.String("input", "", "input script")
	ArgInstanceType         = flag.String("instance-type", "t2.medium", "instance type")
	ArgImageID              = flag.String("image-id", "ami-940cdceb", "image id")
	ArgRoleName             = flag.String("role-name", "", "role name")
	ArgInstanceNum          = flag.Int64("instance-num", 1, "the number of running instance")
	ArgCCompiler            = flag.String("cc", "gcc", "C compiler")
	ArgCXXCompiler          = flag.String("cxx", "g++", "C++ compiler")
)

var (
	VpcTag        = "edcc-netf"
	SubnetTag     = "edcc-subnet-1d"
	IgTag         = "edcc-igw-f"
	SGName        = "edcc-f_SG"
	SGTag         = "edcc-fsg"
	RouteTableTag = "edcc-rtf"
	InstanceTag   = "edcc-AutoConstruct"
	HostTagName   = "IsHost"
	HostTag       = true
	OutputBuild   = "build.sh"
	OutputSetUp   = "setupout.sh"
	UserName      = "ubuntu"
	Distcc        = "distcc"
)

func setup(ins *instance.Instance) (inss []*string, hostPublicIP string, err error) {
	var res *ec2.Reservation
	if res, err = ins.Run(*ArgInstanceNum, OutputSetUp); err != nil {
		return
	} else {
		fmt.Println("Starting instances: ")
		inss = []*string{}
		for _, i := range res.Instances {
			fmt.Printf("\t* %s\n", *i.InstanceId)
			inss = append(inss, aws.String(*i.InstanceId))
		}
		fmt.Printf("Waiting for running...")

		fs := []func() error{
			func() (er error) {
				er = ins.Cli.WaitUntilInstanceRunning(&ec2.DescribeInstancesInput{
					InstanceIds: inss,
				})
				return
			},
			func() (er error) {
				c := &cs3.Cs3{
					Bucket: ArgBucketName,
					Region: ArgRegion,
				}
				er = c.Put(OutputBuild)
				return
			},
		}
		if err = utils.ErrgroupGo(&fs); err != nil {
			return
		}

		var host string
		fs = []func() error{
			func() (er error) {
				if *ArgTopicName != "" && *ArgNotificationEndpoint != "" {
					t := &csns.Topic{
						Topicname:            ArgTopicName,
						NotificationEndpoint: ArgNotificationEndpoint,
					}
					snscli := sns.New(utils.NewSessionFromRegion(*ArgRegion))
					_, er = t.CreateTopicWhenNotExists(snscli)
				}
				return
			},
			func() (er error) {
				describei := &ec2.DescribeInstancesInput{
					Filters: []*ec2.Filter{
						{
							Name:   aws.String("tag:" + HostTagName),
							Values: []*string{aws.String(strconv.FormatBool(HostTag))},
						},
						{
							Name:   aws.String("instance-state-name"),
							Values: []*string{aws.String("running")},
						},
					},
				}
				var out *ec2.DescribeInstancesOutput
				if out, er = ins.Cli.DescribeInstances(describei); err != nil {
					return
				}
				if len(out.Reservations) != 1 {
					er = errors.New("Unexpected instance found or not found")
					return
				}
				if len(out.Reservations[0].Instances) != 1 {
					er = errors.New("Unexpected instance found or not found")
					return
				}
				host = *out.Reservations[0].Instances[0].InstanceId
				hostPublicIP = *out.Reservations[0].Instances[0].PublicIpAddress
				if *ArgRoleName != "" {
					input := &ec2.AssociateIamInstanceProfileInput{
						IamInstanceProfile: &ec2.IamInstanceProfileSpecification{
							Name: aws.String(*ArgRoleName),
						},
						InstanceId: aws.String(host),
					}
					_, er = ins.Cli.AssociateIamInstanceProfile(input)
				}
				return
			},
		}
		if err = utils.ErrgroupGo(&fs); err != nil {
			return
		}

		cl := color.New(color.FgGreen)
		cl.Println("done")
		fmt.Printf("Waiting for returning status ok...")
		if err = ins.Cli.WaitUntilInstanceStatusOk(&ec2.DescribeInstanceStatusInput{
			InstanceIds: []*string{aws.String(host)},
		}); err != nil {
			return
		}
		cl.Println("done")
	}
	return
}

func build(ins *instance.Instance, n *net.Net, sgid, vpcid, subnetid string, inss []*string, hostPublicIP string) (err []error) {
	type ResultSt struct {
		InstanceId string `json:"InstanceId"`
		Status     string `json:"Status"`
		Role       string `json:"Role"`
	}
	fmt.Print("Checking host status...")
	err = []error{}

	sendcmd := func(cmd string) (b []byte, er error) {
		sshc := &cssh.Cssh{
			Ip:          hostPublicIP,
			Port:        "22",
			User:        UserName,
			KeyFilePath: *ArgKeyPath,
		}
		defer sshc.Close()
		if er = sshc.Connect(); er != nil {
			err = append(err, er)
			return
		}
		b, er = sshc.Run(aws.String(cmd))
		return
	}
	sendcmdNoWait := func(cmd string) (er error) {
		sshc := &cssh.Cssh{
			Ip:          hostPublicIP,
			Port:        "22",
			User:        UserName,
			KeyFilePath: *ArgKeyPath,
		}
		defer sshc.Close()
		if er = sshc.Connect(); er != nil {
			err = append(err, er)
			return
		}
		er = sshc.RunotWait(aws.String(cmd))
		return
	}
	cleaning := func(er error) {
		err = append(err, er)
		if er = cleanup(ins, n, sgid, vpcid, subnetid, inss); er != nil {
			err = append(err, er)
		}
	}

	statusCheck := func() (st string, er error) {
		var res []byte
		var rest ResultSt
		if res, er = sendcmd("curl -s localhost"); er != nil {
			fmt.Println("")
			cleaning(er)
			return
		} else {
			if er = json.Unmarshal(res, &rest); er != nil {
				utils.OutErr(errors.New("Failed"))
				cleaning(er)
				return
			} else {
				st = rest.Status
			}
		}
		return
	}

	for {
		if st, er := statusCheck(); er != nil {
			return
		} else {
			if st == "Compiling" {
				break
			}
			fmt.Printf("\n\tStatus: " + st + " (" + time.Now().String() + ")")
			time.Sleep(10 * time.Second)
		}
	}

	green := color.New(color.FgGreen)
	green.Println("\ndone")
	startTime := time.Now()
	color.New(color.FgCyan).Println("\n" + "Starting build: " + startTime.String())

	splited := strings.Split(OutputBuild, "/")
	scriptName := splited[len(splited)-1]
	fmt.Println("Executing - " + scriptName)
	if _, er := sendcmd("sudo cp /root/.bashrc ."); er != nil {
		utils.OutErr(errors.New("Setting script error"))
		cleaning(er)
		return
	}
	if _, er := sendcmd("source .bashrc"); er != nil {
		utils.OutErr(errors.New("Invalid setting"))
		cleaning(er)
		return
	}
	if er := sendcmdNoWait("./" + OutputBuild); er != nil {
		utils.OutErr(errors.New("Build script executing failed."))
		cleaning(er)
		return
	}

	for {
		if st, er := statusCheck(); er != nil {
			return
		} else {
			if st == "BuildSuccess" {
				fin := time.Now()
				green.Printf("\nBuild succeed: ")
				fmt.Println("(" + fin.String() + ")")
				fmt.Println("Duration: " + fin.Sub(startTime).String())
				break
			} else if st == "BuildFailed" {
				utils.OutErr(errors.New("\nBulld failed"))
				break
			}
			fmt.Printf("\n\tStatus: " + st + " (" + time.Now().String() + ")")
		}
	}

	return
}

func cleanup(ins *instance.Instance, n *net.Net, sgid, vpc, subnetid string, instances []*string) (err error) {
    rmf := func(fname *string) (er error) {
        if _, er = os.Stat(*fname); er == nil {
            er = os.Remove(*fname);
        }
        return
    }

    fs := []func()error {
        func() error {
            return rmf(&OutputSetUp)
        },
        func() error {
            return rmf(&OutputBuild)
        },
    }
    utils.ErrgroupGo(&fs)

	cl := color.New(color.FgGreen)

	fmt.Printf("Shutting down...")
	if err = ins.TerminateAndWait(instances); err != nil {
		fmt.Println("")
		return
	}
	cl.Println("done")

	fmt.Printf("Cleanup security group...")
	if err = n.DeleteSecurityGroup(sgid); err != nil {
		fmt.Println("")
		return
	}
	cl.Println("done")

	fmt.Printf("Cleanup subnet...")
	if err = n.DeleteSubnet(subnetid); err != nil {
		fmt.Println("")
		return
	}
	cl.Println("done")

	fmt.Printf("Cleanup gateway...")
	if err = n.DeleteInternetGateways(vpc); err != nil {
		fmt.Println("")
		return
	}
	cl.Println("done")

	fmt.Printf("Cleanup VPC...")
	if err = n.DeleteVpc(vpc); err != nil {
		fmt.Println("")
		return
	}
	cl.Println("done")
	return
}

func init() {
	flag.Parse()
	if *ArgBucketName == "" {
		utils.OutErr(errors.New("must specify bucket name"))
		os.Exit(1)
	} else if *ArgKeyName == "" && *ArgKeyPath == "" && *ArgRoleName == "" {
		utils.OutErr(errors.New("must specify key-name and key-path or role-name"))
		os.Exit(1)
	} else if *ArgKeyName != "" && *ArgKeyPath == "" {
		utils.OutErr(errors.New("must specify key-path"))
		os.Exit(1)
	} else if *ArgKeyPath != "" && *ArgKeyName == "" {
		utils.OutErr(errors.New("must specify key-name"))
		os.Exit(1)
	} else if *ArgInputScript == "" {
		utils.OutErr(errors.New("must specify input-script"))
		os.Exit(1)
	}
}

func main() {
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	if accessKey == "" || secretAccessKey == "" {
        var err error
        if _, err := os.Stat("/home/" + UserName + "/.aws/credentials"); err != nil {
			return
		}
		var f *os.File
        if f, err = os.Open("/home/" + UserName + "/.aws/credentials"); err != nil {
			return
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			text := scanner.Text()
			if strings.HasPrefix(text, "aws_access_key_id") {
				textsp := strings.Split(text, " = ")
				if len(textsp) == 1 {
					return
				}
				accessKey = textsp[1]
			}
			if strings.HasPrefix(text, "aws_secret_access_key") {
				textsp := strings.Split(text, " = ")
				if len(textsp) == 1 {
					return
				}
				secretAccessKey = textsp[1]
			}
		}
	}

	t := &textemplate.Template{
		Region:            ArgRegion,
		InstanceCount:     *ArgInstanceNum * int64(runtime.NumCPU()),
		BucketName:        ArgBucketName,
		TopicName:         ArgTopicName,
		ScriptName:        aws.String(OutputBuild),
		AccessKeyID:       aws.String(accessKey),
		SecretAccessKeyID: aws.String(secretAccessKey),
		CoopTag:           aws.String(InstanceTag),
		Distcc:            aws.String(Distcc + " " + *ArgCCompiler),
		Distcxx:           aws.String(Distcc + " " + *ArgCXXCompiler),
	}
    if err := t.Generate(*ArgSetUpScript, OutputSetUp); err != nil {
		return
	}
	cli := ec2.New(utils.NewSessionFromRegion(*ArgRegion))
	n := &net.Net{
		VpcTagName:             &VpcTag,
		SubnetTagName:          &SubnetTag,
		InternetGatewayTagName: &IgTag,
		SecurityGroupTagName:   &SGTag,
		SecurityGroupName:      &SGName,
		RouteTableTagName:      &RouteTableTag,
		Cli:                    cli,
	}

	if vpcid, subnetid, sgid, err := n.Create(*ArgCidrBlock, *ArgSubnetBlock); !utils.OutErr(err) {
		os.Exit(1)
	} else {
		fmt.Printf("Created VPC: %s\nCreated subnet: %s\nCreated security group: %s\n",
			vpcid, subnetid, sgid)
		ins := &instance.Instance{
			ImageID:                  ArgImageID,
			InstanceType:             ArgInstanceType,
			InstanceTagName:          &InstanceTag,
			HostTagName:              &HostTagName,
			KeyName:                  ArgKeyName,
			SecurityGroupId:          &sgid,
			SubnetId:                 &subnetid,
			AssociatePublicIPAddress: true,
			Cli: cli,
		}
		var inss []*string
		var hostPublicIP string
		if inss, hostPublicIP, err = setup(ins); !utils.OutErr(err) {
			os.Exit(1)
		}

		if errs := build(ins, n, sgid, vpcid, subnetid, inss, hostPublicIP); len(errs) != 0 {
			for _, e := range errs {
				utils.OutErr(e)
			}
			os.Exit(1)
		}

		if err = cleanup(ins, n, sgid, vpcid, subnetid, inss); !utils.OutErr(err) {
			os.Exit(1)
		}
		color.New(color.FgGreen).Println("All OK.")
	}
}
