#!/bin/bash -u

REGION={{.Region}} 
DCC={{.Distcc}}
DCXX={{.Distcxx}}
proc={{.InstanceCount}} 
WORKDIR=workdir
BUCKETNAME={{.BucketName}} 
TOPIC_NAME={{.TopicName}} 

{{.Include_WriteStatus}}

main() {
    mkdir -p workdir
    cd workdir
    CCACHE_DIR=`pwd`/cchache
    mkdir -p ${CCACHE_DIR}
    if [ ! -e ./linux-4.17.12.tar.xz ] && [ ! -e ./linux-4.17.12 ]; then
	    wget https://cdn.kernel.org/pub/linux/kernel/v4.x/linux-4.17.12.tar.xz
    fi
    if [ ! -e ./linux-4.17.12 ]; then
	    tar -xf linux-4.17.12.tar.xz -C ./
	    rm -r ./linux-4.17.12.tar.xz
    fi
    cp `ls /boot/config-* | tail -n 1` ./linux-4.17.12/.config
    cd ./linux-4.17.12
    yes "" | make -j${proc} CC="${DCC}" CXX="${DCXX}" oldconfig > /dev/null 2>&1
    make -j${proc} CC="distcc gcc" CXX="distcc g++"
    Res=$?

    if [ ${Res} -eq 0 ]; then
	    write_status\
	       	"`curl -s http://169.254.169.254/latest/meta-data/instance-id`"\
		    "BuildSuccess"
	    cd ../
	    aws s3 cp ${WORKDIR} s3://${BUCKETNAME}/buildout/ --recursive --region ${REGION}
	    exit 0
    else
	    write_status\
	       	"`curl -s http://169.254.169.254/latest/meta-data/instance-id`"\
		    "BuildFailed"
	    exit ${Res}
    fi
}

main
