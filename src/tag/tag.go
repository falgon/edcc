package tag

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

type Tag struct {
	tinfo []*ec2.Tag
}

func NewTag(tag []*ec2.Tag) *Tag {
	return &Tag{tinfo: tag}
}

func (t *Tag) AddTag(cli *ec2.EC2, resource string) (err error) {
	input := &ec2.CreateTagsInput{
		Resources: []*string{aws.String(resource)},
		Tags:      t.tinfo,
	}
	_, err = cli.CreateTags(input)
	return
}
