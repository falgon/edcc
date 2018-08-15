package textemplate

import (
	"bytes"
	"os"
	"text/template"
)

type Template struct {
	Region            *string
	InstanceCount     int64
	BucketName        *string
	TopicName         *string
	ScriptName        *string
	AccessKeyID       *string
	SecretAccessKeyID *string
	CoopTag           *string
	Distcc            *string
	Distcxx           *string
}

type Member struct {
	Region              string
	InstanceCount       int64
	BucketName          string
	TopicName           string
	ScriptName          string
	AccessKeyID         string
	SecretAccessKeyID   string
	CoopTag             string
	Distcc              string
	Distcxx             string
	Include_WriteStatus string
}

func (_ *Template) intrinsic_func() (code string) {
	code = `is_none() {
        if [ -z "$1" ]; then
            echo true
	    else
		    echo false
	    fi
    }

    send_sns() {
	    if $(is_none "$1"); then
		    exit 1
	    fi
	    TopicArn=$(aws sns list-topics |\
	       	    jq -r ".Topics[]|select(.TopicArn|endswith(\"${TOPIC_NAME}\"))|.TopicArn")
	    if ! $(is_none "${TopicArn}"); then
		    aws sns publish --topic-arn ${TopicArn} --message "$1"
	    else
		    exit 1
	    fi
    	return 0
    }

    selected_instance() {
        selected=$(aws ec2 describe-instances \
            --filters "Name=tag:IsHost,Values=true" \
            --region ${REGION} |\
            jq -r '.Reservations[].Instances[]|select(.State.Name == "running")|.InstanceId')
        if $(is_none "${selected}"); then
            send_sns "failed to get selected instance"
            exit 1
        fi
        echo ${selected}
    }

    write_status() {
        IID=$1
        status=$2
        role=""

        if [ "$(selected_instance)" = "${IID}" ]; then
            role="HOST"
        else
            role="COOP"
        fi

        sudo sh -c \
        "echo \"{ \\\"InstanceId\\\":\\\"$1\\\", \\\"Status\\\": \\\"$2\\\", \\\"Role\\\": \\\"${role}\\\"  }\" >\
                /var/www/html/index.nginx-debian.html"\
                ${IID} ${status} ${role};
    }
    
    write_success() {
	    write_status\
		    "$(curl -s http://169.254.169.254/latest/meta-data/instance-id)"\
		    "BuildSuccess"
    }
    
    write_failed() {
	    write_status\
		    "$(curl -s http://169.254.169.254/latest/meta-data/instance-id)"\
		    "BuildFailed"
    }
    `
	return
}

func (t *Template) Generate(input, outname string) (err error) {
	tt := template.Must(template.ParseFiles(input))
	mem := &Member{
		Region:              *t.Region,
		InstanceCount:       t.InstanceCount,
		BucketName:          *t.BucketName,
		TopicName:           *t.TopicName,
		ScriptName:          *t.ScriptName,
		AccessKeyID:         *t.AccessKeyID,
		SecretAccessKeyID:   *t.SecretAccessKeyID,
		CoopTag:             *t.CoopTag,
		Distcc:              *t.Distcc,
		Distcxx:             *t.Distcxx,
		Include_WriteStatus: t.intrinsic_func(),
	}
	var tpl bytes.Buffer
	if err = tt.Execute(&tpl, mem); err != nil {
		return
	}
	var f *os.File
	if f, err = os.Create(outname); err != nil {
		return
	}
	if err = os.Chmod(outname, 0777); err != nil {
		return
	}
	defer f.Close()
	_, err = f.WriteString(tpl.String())
	return
}
