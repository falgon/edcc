package cs3

import (
	"../utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"os"
	"path/filepath"
)

type Cs3 struct {
	Bucket *string
	Region *string
}

func (c *Cs3) Put(fname string) (err error) {
	var f *os.File
	if f, err = os.Open(fname); err != nil {
		return
	}
	defer f.Close()
	cli := s3manager.NewUploader(utils.NewSessionFromRegion(*c.Region))
	_, err = cli.Upload(&s3manager.UploadInput{
		Bucket: c.Bucket,
		Key:    aws.String(filepath.Base(fname)),
		Body:   f,
	})
	return
}
