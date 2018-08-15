package instance

import (
	"../tag"
	"../utils"
	"encoding/base64"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"math/rand"
	"os"
	"time"
)

type Instance struct {
	ImageID                  *string
	InstanceType             *string
	InstanceTagName          *string
	HostTagName              *string
	KeyName                  *string
	SecurityGroupId          *string
	SubnetId                 *string
	AssociatePublicIPAddress bool
	Cli                      *ec2.EC2
}

func (i *Instance) Run(count int64, userdata string) (res *ec2.Reservation, err error) {
	var out *ec2.CreateLaunchTemplateOutput
	var encoded string
	var host int64

	fs := []func() error{
		func() (er error) {
			var launchTemp *ec2.CreateLaunchTemplateInput
			if *i.KeyName != "" {
				launchTemp = &ec2.CreateLaunchTemplateInput{
					LaunchTemplateData: &ec2.RequestLaunchTemplateData{
						ImageId:      aws.String(*i.ImageID),
						InstanceType: aws.String(*i.InstanceType),
						NetworkInterfaces: []*ec2.LaunchTemplateInstanceNetworkInterfaceSpecificationRequest{
							{
								DeviceIndex:              aws.Int64(0),
								AssociatePublicIpAddress: aws.Bool(true),
								SubnetId:                 aws.String(*i.SubnetId),
								Groups: []*string{
									aws.String(*i.SecurityGroupId),
								},
							},
						},
						TagSpecifications: []*ec2.LaunchTemplateTagSpecificationRequest{
							{
								ResourceType: aws.String("instance"),
								Tags: []*ec2.Tag{
									{
										Key:   aws.String("Name"),
										Value: aws.String(*i.InstanceTagName),
									},
								},
							},
						},
						KeyName: aws.String(*i.KeyName),
					},
					LaunchTemplateName: aws.String("edcc-distcctemplate"),
					VersionDescription: aws.String("1.0.0"),
				}
			} else {
				launchTemp = &ec2.CreateLaunchTemplateInput{
					LaunchTemplateData: &ec2.RequestLaunchTemplateData{
						ImageId:      aws.String(*i.ImageID),
						InstanceType: aws.String(*i.InstanceType),
						NetworkInterfaces: []*ec2.LaunchTemplateInstanceNetworkInterfaceSpecificationRequest{
							{
								DeviceIndex:              aws.Int64(0),
								AssociatePublicIpAddress: aws.Bool(true),
								SubnetId:                 aws.String(*i.SubnetId),
								Groups: []*string{
									aws.String(*i.SecurityGroupId),
								},
							},
						},
						TagSpecifications: []*ec2.LaunchTemplateTagSpecificationRequest{
							{
								ResourceType: aws.String("instance"),
								Tags: []*ec2.Tag{
									{
										Key:   aws.String("Name"),
										Value: aws.String(*i.InstanceTagName),
									},
								},
							},
						},
					},
					LaunchTemplateName: aws.String("edcc-distcctemplate"),
					VersionDescription: aws.String("1.0.0"),
				}
			}
			out, er = i.Cli.CreateLaunchTemplate(launchTemp)
			return
		},
		func() (er error) {
			var file *os.File
			if file, er = os.Open(userdata); er != nil {
				return
			}
			defer file.Close()
			var fi os.FileInfo
			if fi, er = file.Stat(); er != nil {
				return
			}
			data := make([]byte, fi.Size())
			file.Read(data)
			encoded = base64.StdEncoding.EncodeToString(data)
			return
		},
		func() (er error) {
			rand.Seed(time.Now().UnixNano())
			host = rand.Int63n(count)
			return
		},
	}
	if err = utils.ErrgroupGo(&fs); err != nil {
		return
	}

	input := &ec2.RunInstancesInput{
		LaunchTemplate: &ec2.LaunchTemplateSpecification{
			LaunchTemplateId: out.LaunchTemplate.LaunchTemplateId,
		},
		MaxCount: aws.Int64(count),
		MinCount: aws.Int64(count),
		UserData: &encoded,
	}

	if res, err = i.Cli.RunInstances(input); err != nil {
		return
	}

	fs = []func() error{
		func() (er error) {
			deleteLaunchTemp := &ec2.DeleteLaunchTemplateInput{
				LaunchTemplateId: out.LaunchTemplate.LaunchTemplateId,
			}
			_, er = i.Cli.DeleteLaunchTemplate(deleteLaunchTemp)
			return
		},
		func() (er error) {
			kv := &ec2.Tag{Key: aws.String(*i.HostTagName), Value: aws.String("true")}
			t := tag.NewTag([]*ec2.Tag{kv})
			er = t.AddTag(i.Cli, *res.Instances[host].InstanceId)
			return
		},
	}
	err = utils.ErrgroupGo(&fs)
	return
}

func (i *Instance) TerminateAndWait(instanceids []*string) (err error) {
	if _, err = i.Cli.TerminateInstances(&ec2.TerminateInstancesInput{InstanceIds: instanceids}); err == nil {
		err = i.Cli.WaitUntilInstanceTerminated(&ec2.DescribeInstancesInput{InstanceIds: instanceids})
	}
	return
}
