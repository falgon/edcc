package csns

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sns"
	"strings"
)

type Topic struct {
	Topicname            *string
	NotificationEndpoint *string
}

func (t *Topic) createSubscribe(cli *sns.SNS, topicarn string) (subscribearn string, err error) {
	input := &sns.SubscribeInput{
		TopicArn: aws.String(topicarn),
		Endpoint: t.NotificationEndpoint,
		Protocol: aws.String("email"),
	}

	var out *sns.SubscribeOutput
	if out, err = cli.Subscribe(input); err == nil {
		subscribearn = *out.SubscriptionArn
	}
	return
}

func (t *Topic) CreateTopicWhenNotExists(cli *sns.SNS) (subscription string, err error) {
	tpinput := &sns.ListTopicsInput{}
	var out *sns.ListTopicsOutput
	if out, err = cli.ListTopics(tpinput); err != nil {
		return
	}

	var topicarn string
	for _, tp := range out.Topics {
		ss := strings.Split(*tp.TopicArn, ":")
		if ss[len(ss)-1] == *t.Topicname {
			topicarn = *tp.TopicArn
			break
		}
	}

	if topicarn != "" {
		tpbtinput := &sns.ListSubscriptionsByTopicInput{
			TopicArn: &topicarn,
		}
		var lbtout *sns.ListSubscriptionsByTopicOutput
		if lbtout, err = cli.ListSubscriptionsByTopic(tpbtinput); err != nil {
			return
		}
		for _, lbt := range lbtout.Subscriptions {
			if *lbt.TopicArn == topicarn {
				subscription = *lbt.SubscriptionArn
				break
			}
		}
		if subscription == "" {
			subscription, err = t.createSubscribe(cli, topicarn)
		}
	} else {
		ctinput := &sns.CreateTopicInput{
			Name: t.Topicname,
		}
		var cto *sns.CreateTopicOutput
		if cto, err = cli.CreateTopic(ctinput); err != nil {
			return
		}
		subscription, err = t.createSubscribe(cli, *cto.TopicArn)
	}
	return
}
