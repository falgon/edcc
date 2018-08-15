#!/bin/bash -e

REGION={{.Region}}
AWS_DISTCC_VALUE={{.CoopTag}} 
BUCKETNAME={{.BucketName}} 
ACCESSKEYID={{.AccessKeyID}} 
SECRET_ACCESSKEY_ID={{.SecretAccessKeyID}} 
DOWNFILE={{.ScriptName}} 
TOPIC_NAME={{.TopicName}} 

AWS_DISTCC_TAG=Name
QCURL="curl -s"
APTGET=apt-get 
CCACHE_DIR=`pwd`/cchache

chkprevres() {
    Res=$1
	if [ ${Res} -ne 0 ]; then
		exit 1
	fi	
}

if [ -z "`cat /etc/hosts | grep $(hostname)`" ]; then
	sudo sh -c 'echo 127.0.1.1 $(hostname) >> /etc/hosts'> /dev/null 2>&1
fi
sudo ${APTGET} update -qq
sudo ${APTGET} -y -qq upgrade 
sudo ${APTGET} -y -qq dist-upgrade
sudo ${APTGET} -y -qq install build-essential \
	checkinstall \
	software-properties-common \
	git \
	fakeroot \
	ncurses-dev \
	xz-utils \
	libssl-dev \
	bc \
	flex \
	libelf-dev \
	bison \
	jq \
	python-pip \
	nginx
chkprevres $?
pip install -q awscli
chkprevres $?
aws configure set aws_access_key_id ${ACCESSKEYID}
chkprevres $?
aws configure set aws_secret_access_key ${SECRET_ACCESSKEY_ID}
chkprevres $?
cd ~/
chkprevres $?
sudo cp /usr/share/zoneinfo/Asia/Tokyo /etc/localtime
chkprevres $?
export AWS_DEFAULT_REGION=${REGION}
chkprevres $?
echo "export AWS_DEFAULT_REGION=${REGION}" >> ~/.bashrc
chkprevres $?

{{.Include_WriteStatus}}

get_group_pia() {
	DH=`aws ec2 describe-instances\
		--filter "Name=tag:${AWS_DISTCC_TAG},Values=${AWS_DISTCC_VALUE}"\
	       	--region ${REGION} |\
		jq '.Reservations[].Instances[]|(select(.State.Name == "running"))|.PrivateIpAddress' |\
		sed -e "s/[\r\n]\+/ /g" |\
	       	sed -e 's/"//g'`
	
	if `is_none "${DH}"`; then
		send_sns "failed to get distcc private ip addresses"
		exit 1
	fi
	echo ${DH}
}

ami_host() {
	selected_instanceid=`selected_instance`
	IID=`${QCURL} http://169.254.169.254/latest/meta-data/instance-id`
	if [ ${selected_instanceid} = ${IID} ]; then
		echo true
	else
		echo false
	fi
}

get_tools() {
	this_instanceid=$1
	RETRYMAX=$2
	write_status ${this_instanceid} "Initializing"

	sudo add-apt-repository ppa:ubuntu-toolchain-r/test -y
	sudo ${APTGET} update -qq
	sudo ${APTGET} install gcc-8 g++-8 -y -qq
	sudo update-alternatives\
 		--install /usr/bin/gcc gcc /usr/bin/gcc-8 60\
 		--slave /usr/bin/g++ g++ /usr/bin/g++-8
	sudo update-alternatives\
 		--config gcc
	if [ -z "`g++ --version | head -n 1 | grep 8.1.0`" ]; then
		send_sns "Failed to configuration to gcc"
		write_status  ${this_instanceid} "Failed"
		exit 1
	fi
	
	sudo ${APTGET} -y -qq install distcc ccache
	sudo ${APTGET} -y -qq autoremove

	DOWNFILEPATH=/home/ubuntu

	selected_instanceid=`selected_instance`
	sudo systemctl restart nginx
	if [ "${selected_instanceid}" = "${this_instanceid}" ]; then
		S3RESULT=""
		for ((i=0; i<${RETRYMAX}; i++)) do
		       	S3RESULT=`aws s3 cp s3://${BUCKETNAME}/${DOWNFILE} ${DOWNFILEPATH}`
			if ! `is_none $(echo ${S3RESULT} | grep download)`; then
				break
			fi
		done
		if [ ! -e ${DOWNFILEPATH}/${DOWNFILE} ]; then
			send_sns "Failed to download from s3: ${S3RESULT}"
			write_status ${this_instanceid} "Failed"
			exit 1
		fi
		sudo chmod +x ${DOWNFILEPATH}/${DOWNFILE}
	fi
}

setup_distcc() {
	if [ -z "`cat ~/.bashrc | grep CCACHE_DIR`" ]; then
		echo "export CCACHE_DIR=${CCACHE_DIR}" >> ~/.bashrc
	fi
	if [ -z "`cat ~/.bashrc | grep USE_CCACHE`" ]; then
		echo "export USE_CCACHE=1" >> ~/.bashrc
	fi
	if [ -z "`cat ~/.bashrc | grep DISTCC_VERBOSE`" ]; then
		echo "export DISTCC_VERBOSE=0" >> ~/.bashrc
	fi
	if [ ! -e ${CCACHE_DIR} ]; then 
		mkdir -p ${CCACHE_DIR}
	fi

	IID=$1
	DNIC=`aws ec2 describe-instances --region ${REGION} --instance-ids ${IID} |\
		jq -r '.Reservations[].Instances[]|select(.State.Name == "running")|.PrivateIpAddress'`
	VPCID=` aws ec2 describe-instances --instance-ids ${IID} --region ${REGION} |\
	       	jq -r '.Reservations[].Instances[].VpcId'`
	CIDR=`aws ec2 describe-vpcs --vpc-ids ${VPCID} --region ${REGION} |\
		jq -r '.Vpcs[].CidrBlock'`
	
	if `is_none "${IID}"` ; then
		send_sns "Failed on setup_distcc: failed to get this instance id"
		write_status ${IID} "Failed"
		exit 1
	elif `is_none "${DNIC}"` ; then
		send_sns "Failed to setup_distcc: failed to get private ip addresses"
		write_status ${IID} "Failed"
		exit 1
	elif `is_none "${VPCID}"` ; then
		send_sns "Failed to setup_distcc: failed to get VpcId"a
		write_status ${IID} "Failed"
		exit 1
	elif `is_none "${CIDR}"` ; then
		send_sns "Failed to setup_distcc: failed to get CidrBlock"
		write_status ${IID} "Failed"
		exit 1
	fi
	
	sudo sh -c 'echo LISTENER=\"$0\" > /etc/default/distcc' ${DNIC}
	sudo sh -c 'echo STARTDISTCC=\"true\" >> /etc/default/distcc'
	sudo sh -c 'echo -n ALLOWEDNETS=\"127.0.0.1 >> /etc/default/distcc'
       	sudo sh -c 'echo " $0\"" >> /etc/default/distcc' ${CIDR}
	sudo sh -c 'echo NICE=\"10\" >> /etc/default/distcc'
	sudo sh -c 'echo JOBS=\"\" >> /etc/default/distcc'
	sudo sh -c 'echo ZEROCONF=\"true\" >> /etc/default/distcc'
	
	if [ -z "`cat ~/.bashrc | grep DISTCC_HOSTS`" ]; then
		DH=`echo localhost $(get_group_pia)`
		echo "export DISTCC_HOSTS=\"${DH}\"" >> ~/.bashrc
	fi
	export USE_CCACHE=1
	export DISTCC_VERBOSE=0

	write_status ${IID} "Ready"
}

do_compile() {
	if `is_none "$1"`; then
		send_sns "Failed to do_compile: must be set is_host flag"
		write_status ${IID} "Failed"
		exit 1
	elif `is_none "$2"`; then
		send_sns "Failed to do_compile: failed to get this instance id"
		write_status ${IID} "Failed"
		exit 1
	elif `is_none "$3"`; then
		send_ans "Failed to do_compile: must be set retry max"
		write_status ${IID} "Failed"
		exit 1
	fi	

	IID=$2
	RETRYMAX=$3
	DURATION=$4
	
	if $1; then
		write_status ${IID} "Compiling"
		DH=`get_group_pia`
		declare -a VIPS=($(echo ${DH}))
		for vip in ${VIPS[@]}; do
			RESPONSE=""
			# if i = RETRYMAX - 1, its vip is not join compile
			for ((i=0; i<${RETRYMAX}; i++)); do
				if [ "${RESPONSE}" = "Ready" ] || [ "${RESPONSE}" = "Compiling" ] ; then
					send_sns "${vip} success"
					break
				elif [ "${RESPONSE}" = "Failed" ]; then
					break
				fi	  
				sleep ${DURATION}
				RESPONSE=`${QCURL} ${vip} | jq -r '.Status'`
			done
		done
	fi
}

main() {
	IID=`${QCURL} http://169.254.169.254/latest/meta-data/instance-id`
	if `is_none ${IID}`; then
		echo "Failed to get this instnace id"
		exit 1
	fi
	
	get_tools ${IID} 3
	setup_distcc ${IID}
	source ~/.bashrc
	sudo systemctl restart distcc
	do_compile `ami_host` ${IID} 10 5
}

main
