package apimodel

import (
	"github.com/ringoid/commons"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/kinesis"
	"github.com/aws/aws-sdk-go/service/firehose"
	"os"
	"github.com/aws/aws-sdk-go/aws/session"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
)

var Anlogger *commons.Logger
var InternalAuthFunctionName string
var GetNewFacesFunctionName string
var LikesYouFunctionName string
var MatchesFunctionName string
var ClientLambda *lambda.Lambda
var CommonStreamName string
var AwsKinesisClient *kinesis.Kinesis
var DeliveryStreamName string
var AwsDeliveryStreamClient *firehose.Firehose
var GetNewImagesInternalFunctionName string

var BaseCloudWatchNamespace string
var NewFaceProfilesReturnMetricName string
var LikesYouProfilesReturnMetricName string
var MatchProfilesReturnMetricName string
var MessageProfilesReturnMetricName string

var AwsCWClient *cloudwatch.CloudWatch

func InitLambdaVars(lambdaName string) {
	var env string
	var ok bool
	var papertrailAddress string
	var err error
	var awsSession *session.Session

	env, ok = os.LookupEnv("ENV")
	if !ok {
		fmt.Printf("lambda-initialization : service_common.go : env can not be empty ENV\n")
		os.Exit(1)
	}
	fmt.Printf("lambda-initialization : service_common.go : start with ENV = [%s]\n", env)

	papertrailAddress, ok = os.LookupEnv("PAPERTRAIL_LOG_ADDRESS")
	if !ok {
		fmt.Printf("lambda-initialization : service_common.go : env can not be empty PAPERTRAIL_LOG_ADDRESS\n")
		os.Exit(1)
	}
	fmt.Printf("lambda-initialization : service_common.go : start with PAPERTRAIL_LOG_ADDRESS = [%s]\n", papertrailAddress)

	Anlogger, err = commons.New(papertrailAddress, fmt.Sprintf("%s-%s", env, lambdaName))
	if err != nil {
		fmt.Errorf("lambda-initialization : service_common.go : error during startup : %v\n", err)
		os.Exit(1)
	}
	Anlogger.Debugf(nil, "lambda-initialization : service_common.go : logger was successfully initialized")

	InternalAuthFunctionName, ok = os.LookupEnv("INTERNAL_AUTH_FUNCTION_NAME")
	if !ok {
		Anlogger.Fatalf(nil, "lambda-initialization : service_common.go : env can not be empty INTERNAL_AUTH_FUNCTION_NAME")
	}
	Anlogger.Debugf(nil, "lambda-initialization : service_common.go : start with INTERNAL_AUTH_FUNCTION_NAME = [%s]", InternalAuthFunctionName)

	GetNewFacesFunctionName, ok = os.LookupEnv("INTERNAL_GET_NEW_FACES_FUNCTION_NAME")
	if !ok {
		Anlogger.Fatalf(nil, "lambda-initialization : service_common.go : env can not be empty INTERNAL_GET_NEW_FACES_FUNCTION_NAME")
	}
	Anlogger.Debugf(nil, "lambda-initialization : service_common.go : start with INTERNAL_GET_NEW_FACES_FUNCTION_NAME = [%s]", GetNewFacesFunctionName)

	LikesYouFunctionName, ok = os.LookupEnv("INTERNAL_LIKES_YOU_FUNCTION_NAME")
	if !ok {
		Anlogger.Fatalf(nil, "lambda-initialization : service_common.go : env can not be empty INTERNAL_LIKES_YOU_FUNCTION_NAME")
	}
	Anlogger.Debugf(nil, "lambda-initialization : service_common.go : start with INTERNAL_LIKES_YOU_FUNCTION_NAME = [%s]", LikesYouFunctionName)

	MatchesFunctionName, ok = os.LookupEnv("INTERNAL_MATCHES_FUNCTION_NAME")
	if !ok {
		Anlogger.Fatalf(nil, "lambda-initialization : service_common.go : env can not be empty INTERNAL_MATCHES_FUNCTION_NAME")
	}
	Anlogger.Debugf(nil, "lambda-initialization : service_common.go : start with INTERNAL_MATCHES_FUNCTION_NAME = [%s]", MatchesFunctionName)

	GetNewImagesInternalFunctionName, ok = os.LookupEnv("INTERNAL_GET_NEW_IMAGES_FUNCTION_NAME")
	if !ok {
		Anlogger.Fatalf(nil, "lambda-initialization : service_common.go : env can not be empty INTERNAL_GET_NEW_IMAGES_FUNCTION_NAME")
	}
	Anlogger.Debugf(nil, "lambda-initialization : service_common.go : start with INTERNAL_GET_NEW_IMAGES_FUNCTION_NAME = [%s]", GetNewImagesInternalFunctionName)

	CommonStreamName, ok = os.LookupEnv("COMMON_STREAM")
	if !ok {
		Anlogger.Fatalf(nil, "lambda-initialization : service_common.go : env can not be empty COMMON_STREAM")
		os.Exit(1)
	}
	Anlogger.Debugf(nil, "lambda-initialization : service_common.go : start with COMMON_STREAM = [%s]", CommonStreamName)

	DeliveryStreamName, ok = os.LookupEnv("DELIVERY_STREAM")
	if !ok {
		Anlogger.Fatalf(nil, "lambda-initialization : service_common.go : env can not be empty DELIVERY_STREAM")
		os.Exit(1)
	}
	Anlogger.Debugf(nil, "lambda-initialization : service_common.go : start with DELIVERY_STREAM = [%s]", DeliveryStreamName)

	BaseCloudWatchNamespace, ok = os.LookupEnv("BASE_CLOUD_WATCH_NAMESPACE")
	if !ok {
		Anlogger.Fatalf(nil, "lambda-initialization : service_common.go : env can not be empty BASE_CLOUD_WATCH_NAMESPACE")
	}
	Anlogger.Debugf(nil, "lambda-initialization : service_common.go : start with BASE_CLOUD_WATCH_NAMESPACE = [%s]", BaseCloudWatchNamespace)

	NewFaceProfilesReturnMetricName, ok = os.LookupEnv("CLOUD_WATCH_NEW_FACES_RETURN")
	if !ok {
		Anlogger.Fatalf(nil, "lambda-initialization : service_common.go : env can not be empty CLOUD_WATCH_NEW_FACES_RETURN")
	}
	Anlogger.Debugf(nil, "lambda-initialization : service_common.go : start with CLOUD_WATCH_NEW_FACES_RETURN = [%s]", NewFaceProfilesReturnMetricName)

	LikesYouProfilesReturnMetricName, ok = os.LookupEnv("CLOUD_WATCH_LIKES_YOU_RETURN")
	if !ok {
		Anlogger.Fatalf(nil, "lambda-initialization : service_common.go : env can not be empty CLOUD_WATCH_LIKES_YOU_RETURN")
	}
	Anlogger.Debugf(nil, "lambda-initialization : service_common.go : start with CLOUD_WATCH_LIKES_YOU_RETURN = [%s]", LikesYouProfilesReturnMetricName)

	MatchProfilesReturnMetricName, ok = os.LookupEnv("CLOUD_WATCH_MATCHES_RETURN")
	if !ok {
		Anlogger.Fatalf(nil, "lambda-initialization : service_common.go : env can not be empty CLOUD_WATCH_MATCHES_RETURN")
	}
	Anlogger.Debugf(nil, "lambda-initialization : service_common.go : start with CLOUD_WATCH_MATCHES_RETURN = [%s]", MatchProfilesReturnMetricName)

	MessageProfilesReturnMetricName, ok = os.LookupEnv("CLOUD_WATCH_MESSAGES_RETURN")
	if !ok {
		Anlogger.Fatalf(nil, "lambda-initialization : service_common.go : env can not be empty CLOUD_WATCH_MESSAGES_RETURN")
	}
	Anlogger.Debugf(nil, "lambda-initialization : service_common.go : start with CLOUD_WATCH_MESSAGES_RETURN = [%s]", MessageProfilesReturnMetricName)

	awsSession, err = session.NewSession(aws.NewConfig().
		WithRegion(commons.Region).WithMaxRetries(commons.MaxRetries).
		WithLogger(aws.LoggerFunc(func(args ...interface{}) { Anlogger.AwsLog(args) })).WithLogLevel(aws.LogOff))
	if err != nil {
		Anlogger.Fatalf(nil, "lambda-initialization : service_common.go : error during initialization : %v", err)
	}
	Anlogger.Debugf(nil, "lambda-initialization : service_common.go : aws session was successfully initialized")

	ClientLambda = lambda.New(awsSession)
	Anlogger.Debugf(nil, "lambda-initialization : service_common.go : lambda client was successfully initialized")

	AwsKinesisClient = kinesis.New(awsSession)
	Anlogger.Debugf(nil, "lambda-initialization : service_common.go : kinesis client was successfully initialized")

	AwsDeliveryStreamClient = firehose.New(awsSession)
	Anlogger.Debugf(nil, "lambda-initialization : service_common.go : firehose client was successfully initialized")

	AwsCWClient = cloudwatch.New(awsSession)
	Anlogger.Debugf(nil, "lambda-initialization : service_common.go : cloudwatch client was successfully initialized")
}

func MarkNewFacesDefaultSort(userId string, resp *GetNewFacesFeedResp, lc *lambdacontext.LambdaContext) *GetNewFacesFeedResp {
	Anlogger.Debugf(lc, "service_common.go : mark new faces resp by default sort for userId [%s]", userId)
	for index := range resp.Profiles {
		resp.Profiles[index].DefaultSortingOrderPosition = index
	}
	Anlogger.Debugf(lc, "service_common.go : successfully mark new faces resp by default sort for userId [%s]", userId)
	return resp
}

func MarkLMMDefaultSort(userId string, resp *LMMFeedResp, lc *lambdacontext.LambdaContext) *LMMFeedResp {
	Anlogger.Debugf(lc, "service_common.go : mark lmm resp by default sort for userId [%s]", userId)
	for index := range resp.LikesYou {
		resp.LikesYou[index].DefaultSortingOrderPosition = index
	}
	for index := range resp.Matches {
		resp.Matches[index].DefaultSortingOrderPosition = index
	}
	Anlogger.Debugf(lc, "service_common.go : successfully mark llm resp by default sort for userId [%s]", userId)
	return resp
}
