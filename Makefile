GO=go
DST=dst
OUT=edcc

all: build

build:
	@mkdir -p $(DST)
	@$(GO) build -o $(DST)/$(OUT) ./src/main.go

run:
	@./$(DST)/$(OUT) \
		-region="ap-northeast-1" \
		-bucket-name="bucketname" \
		-cidr="10.0.0.0/16"\
		-subnet="10.0.0.0/24" \
		-image-id="ami-940cdceb" \
		-instance-type="t2.medium" \
		-key-name="keyname" \
		-key-path="keypath.pem" \
		-topic-name="topicname" \
		-notification-endpoint="<email_address>" \
		-setup-script="../src/setup.sh" \
		-input="../build_script_example/simple.sh" \
		-instance-num=2

get:
	@go get golang.org/x/sync/errgroup\
		github.com/aws/aws-sdk-go/aws\
		github.com/aws/aws-sdk-go/aws/session\
		github.com/aws/aws-sdk-go/service/s3\
		github.com/aws/aws-sdk-go/service/s3/s3manager\
		github.com/aws/aws-sdk-go/service/sns\
		golang.org/x/sync/errgroup\
		golang.org/x/crypto/ssh\
		github.com/fatih/color
	

help:
	./$(DST)/$(OUT) --help

clean:
	@$(RM) -rf dst
