#!/bin/bash -u

REGION={{.Region}} 
proc={{.InstanceCount}} 
WORKDIR=workdir
BUCKETNAME={{.BucketName}} 
TOPIC_NAME={{.TopicName}} 
DCC={{.Distcc}}
DCXX={{.Distcxx}} 

CCACHE_DIR=`pwd`/cchache

{{.Include_WriteStatus}}

main() {
    mkdir -p ${WORKDIR}
    cd ${WORKDIR}

    mkdir -p ${CCACHE_DIR}
    echo -e "#include <iostream>\nint main(){std::cout << \"hoge\" << std::endl;}" > hoge.cpp
    export CCACHE_DIR=${CCACHE_DIR}
    ${DCXX} hoge.cpp -o out 
    Res=$?

    if [ ${Res} -eq 0 ]; then
        {{.write_success}}
	    aws s3 cp --region ${REGION} out s3://${BUCKETNAME}/
	    exit 0
    else
        {{.write_failed}}
	    exit ${Res}
    fi
}

main
