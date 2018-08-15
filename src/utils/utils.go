package utils

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/fatih/color"
	"golang.org/x/sync/errgroup"
	"os"
)

func NewSessionFromRegion(region string) *session.Session {
	return session.Must(session.NewSession(&aws.Config{
		Region: aws.String(region),
	}))
}

func GetThisInstanceId(region string) (r string, err error) {
	var doc ec2metadata.EC2InstanceIdentityDocument
	if doc, err = ec2metadata.New(NewSessionFromRegion(region)).GetInstanceIdentityDocument(); err == nil {
		r = doc.InstanceID
	}
	return
}

func OutErr(err error) bool {
	if err != nil {
		cl := color.New(color.FgRed)
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				cl.Fprintf(os.Stderr, aerr.Error()+"\n")
			}
		} else {
			cl.Fprintf(os.Stderr, err.Error()+"\n")
		}
		return false
	}
	return true
}

func ErrgroupGo(fs *[]func() error) (err error) {
	eg := errgroup.Group{}
	for _, fn := range *fs {
		eg.Go(fn)
	}
	err = eg.Wait()
	return
}
