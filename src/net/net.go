package net

import (
	"../tag"
	"../utils"
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"golang.org/x/sync/errgroup"
)

type Net struct {
	Cli                    *ec2.EC2
	VpcTagName             *string
	SubnetTagName          *string
	InternetGatewayTagName *string
	SecurityGroupTagName   *string
	SecurityGroupName      *string
	RouteTableTagName      *string
}

func (n *Net) createVpc(cidrBlock string) (vpcid string, err error) {
	input := &ec2.CreateVpcInput{
		CidrBlock: aws.String(cidrBlock),
	}
	var id *ec2.CreateVpcOutput
	if id, err = n.Cli.CreateVpc(input); err == nil {
		vpcid = *id.Vpc.VpcId

		kv := &ec2.Tag{Key: aws.String("Name"), Value: aws.String(*n.VpcTagName)}
		t := tag.NewTag([]*ec2.Tag{kv})
		err = t.AddTag(n.Cli, vpcid)
	}
	return
}

func (n *Net) createSubnet(vpcBlock, subnetBlock string) (vpcid, subnetid string, err error) {
	if vpcid, err = n.createVpc(vpcBlock); err == nil {
		input := &ec2.CreateSubnetInput{
			CidrBlock: aws.String(subnetBlock),
			VpcId:     aws.String(vpcid),
		}
		var id *ec2.CreateSubnetOutput
		if id, err = n.Cli.CreateSubnet(input); err == nil {
			subnetid = *id.Subnet.SubnetId

			kv := &ec2.Tag{Key: aws.String("Name"), Value: aws.String(*n.SubnetTagName)}
			t := tag.NewTag([]*ec2.Tag{kv})
			err = t.AddTag(n.Cli, subnetid)
		}
	}
	return
}

func (n *Net) createInternetGateway() (internetgatewayid string, err error) {
	input := &ec2.CreateInternetGatewayInput{}
	var id *ec2.CreateInternetGatewayOutput
	if id, err = n.Cli.CreateInternetGateway(input); err == nil {
		internetgatewayid = *id.InternetGateway.InternetGatewayId

		kv := &ec2.Tag{Key: aws.String("Name"), Value: aws.String(*n.InternetGatewayTagName)}
		t := tag.NewTag([]*ec2.Tag{kv})
		err = t.AddTag(n.Cli, internetgatewayid)
	}
	return
}

func (n *Net) attachInternetGateway(internetgatewayid, vpcid string) (err error) {
	input := &ec2.AttachInternetGatewayInput{
		InternetGatewayId: aws.String(internetgatewayid),
		VpcId:             aws.String(vpcid),
	}
	_, err = n.Cli.AttachInternetGateway(input)
	return
}

func (n *Net) createRoute(vpcBlock, subnetBlock string) (vpcid, subnetid string, err error) {
	var internetgatewayid string

	fs := []func() error{
		func() (er error) {
			vpcid, subnetid, er = n.createSubnet(vpcBlock, subnetBlock)
			return er
		},
		func() (er error) {
			internetgatewayid, er = n.createInternetGateway()
			return er
		},
	}
	if err = utils.ErrgroupGo(&fs); err != nil {
		return
	}
	if err = n.attachInternetGateway(internetgatewayid, vpcid); err != nil {
		return
	}

	rtinput := &ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(vpcid)},
			},
		},
	}
	var routetableid string
	var id *ec2.DescribeRouteTablesOutput
	if id, err = n.Cli.DescribeRouteTables(rtinput); err == nil {
		if len(id.RouteTables) != 1 {
			err = errors.New("Found the unexpected route table")
			return
		} else {
			routetableid = *id.RouteTables[0].RouteTableId
		}

		kv := &ec2.Tag{Key: aws.String("Name"), Value: aws.String(*n.RouteTableTagName)}
		t := tag.NewTag([]*ec2.Tag{kv})
		err = t.AddTag(n.Cli, routetableid)
	} else {
		return
	}

	rinput := &ec2.CreateRouteInput{
		RouteTableId:         aws.String(routetableid),
		GatewayId:            aws.String(internetgatewayid),
		DestinationCidrBlock: aws.String("0.0.0.0/0"),
	}
	var response *ec2.CreateRouteOutput
	if response, err = n.Cli.CreateRoute(rinput); err != nil {
		if !*response.Return {
			err = errors.New("Net.createRoute: CreateRoute API operation was succeeds, but the request was failed.")
			return
		}
	}

	asinput := &ec2.AssociateRouteTableInput{
		RouteTableId: aws.String(routetableid),
		SubnetId:     aws.String(subnetid),
	}
	_, err = n.Cli.AssociateRouteTable(asinput)
	return
}

func (n *Net) createSecurityGroup(vpcid, cidrBlock string) (securitygroupid string, err error) {
	csinput := &ec2.CreateSecurityGroupInput{
		Description: aws.String("http, distcc"),
		GroupName:   aws.String(*n.SecurityGroupName),
		VpcId:       aws.String(vpcid),
	}

	var id *ec2.CreateSecurityGroupOutput
	if id, err = n.Cli.CreateSecurityGroup(csinput); err == nil {
		securitygroupid = *id.GroupId
	} else {
		return
	}

	asgi := func(sgid string, port int64, cidrBlock string) (err error) {
		asinput := &ec2.AuthorizeSecurityGroupIngressInput{
			GroupId: aws.String(securitygroupid),
			IpPermissions: []*ec2.IpPermission{
				{
					FromPort:   aws.Int64(port),
					ToPort:     aws.Int64(port),
					IpProtocol: aws.String("tcp"),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String(cidrBlock),
						},
					},
				},
			},
		}
		_, err = n.Cli.AuthorizeSecurityGroupIngress(asinput)
		return
	}
	type arg struct {
		Port      int64
		CidrBlock string
	}
	args := []*arg{
		&arg{
			Port:      22,
			CidrBlock: "0.0.0.0/0",
		},
		&arg{
			Port:      80,
			CidrBlock: cidrBlock,
		},
		&arg{
			Port:      3632,
			CidrBlock: cidrBlock,
		},
	}

	eg := errgroup.Group{}
	for _, a := range args {
		a := a
		eg.Go(func() error { return asgi(securitygroupid, a.Port, a.CidrBlock) })
	}
	eg.Go(func() error {
		kv := &ec2.Tag{Key: aws.String("Name"), Value: aws.String(*n.SecurityGroupTagName)}
		t := tag.NewTag([]*ec2.Tag{kv})
		return t.AddTag(n.Cli, securitygroupid)
	})

	err = eg.Wait()
	return
}

func (n *Net) Create(vpcBlock, subnetBlock string) (vpcid, subnetid, sgid string, err error) {
	if vpcid, subnetid, err = n.createRoute(vpcBlock, subnetBlock); err == nil {
		sgid, err = n.createSecurityGroup(vpcid, subnetBlock)
		return
	}
	return
}

func (n *Net) DeleteSecurityGroup(sgid string) (err error) {
	_, err = n.Cli.DeleteSecurityGroup(&ec2.DeleteSecurityGroupInput{GroupId: aws.String(sgid)})
	return
}

func (n *Net) DeleteSubnet(subnetid string) (err error) {
	_, err = n.Cli.DeleteSubnet(&ec2.DeleteSubnetInput{SubnetId: aws.String(subnetid)})
	return
}

func (n *Net) DeleteRouteTable(routetableid string) (err error) {
	_, err = n.Cli.DeleteRouteTable(&ec2.DeleteRouteTableInput{RouteTableId: aws.String(routetableid)})
	return
}

func (n *Net) DeleteInternetGateways(vpcid string) (err error) {
	var res *ec2.DescribeInternetGatewaysOutput
	if res, err = n.Cli.DescribeInternetGateways(&ec2.DescribeInternetGatewaysInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("attachment.vpc-id"),
				Values: []*string{
					aws.String(vpcid),
				},
			},
		},
	}); err == nil {
		if len(res.InternetGateways) != 1 {
			err = errors.New("Unexpected gateway found or not found")
			return
		}
		if _, err = n.Cli.DetachInternetGateway(&ec2.DetachInternetGatewayInput{
			InternetGatewayId: res.InternetGateways[0].InternetGatewayId,
			VpcId:             aws.String(vpcid),
		}); err != nil {
			return
		}
		_, err = n.Cli.DeleteInternetGateway(&ec2.DeleteInternetGatewayInput{
			InternetGatewayId: res.InternetGateways[0].InternetGatewayId,
		})
	}
	return
}

func (n *Net) DeleteVpc(vpcid string) (err error) {
	_, err = n.Cli.DeleteVpc(&ec2.DeleteVpcInput{VpcId: aws.String(vpcid)})
	return
}
