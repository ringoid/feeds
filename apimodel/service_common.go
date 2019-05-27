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
	"strings"
)

var Anlogger *commons.Logger
var InternalAuthFunctionName string
var GetNewFacesFunctionName string
var LikesYouFunctionName string
var MatchesFunctionName string
var MessagesFunctionName string
var LMHISFunctionName string
var ClientLambda *lambda.Lambda
var CommonStreamName string
var AwsKinesisClient *kinesis.Kinesis
var DeliveryStreamName string
var AwsDeliveryStreamClient *firehose.Firehose

var BaseCloudWatchNamespace string
var NewFaceProfilesReturnMetricName string
var LikesYouProfilesReturnMetricName string
var MatchProfilesReturnMetricName string
var MessageProfilesReturnMetricName string

var AwsCWClient *cloudwatch.CloudWatch
var Env string

var userIdStatusEnabledMap map[string]bool

func InitLambdaVars(lambdaName string) {
	var ok bool
	var papertrailAddress string
	var err error
	var awsSession *session.Session

	Env, ok = os.LookupEnv("ENV")
	if !ok {
		fmt.Printf("lambda-initialization : service_common.go : env can not be empty ENV\n")
		os.Exit(1)
	}
	fmt.Printf("lambda-initialization : service_common.go : start with ENV = [%s]\n", Env)

	papertrailAddress, ok = os.LookupEnv("PAPERTRAIL_LOG_ADDRESS")
	if !ok {
		fmt.Printf("lambda-initialization : service_common.go : env can not be empty PAPERTRAIL_LOG_ADDRESS\n")
		os.Exit(1)
	}
	fmt.Printf("lambda-initialization : service_common.go : start with PAPERTRAIL_LOG_ADDRESS = [%s]\n", papertrailAddress)

	Anlogger, err = commons.New(papertrailAddress, fmt.Sprintf("%s-%s", Env, lambdaName))
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

	MessagesFunctionName, ok = os.LookupEnv("INTERNAL_MESSAGES_FUNCTION_NAME")
	if !ok {
		Anlogger.Fatalf(nil, "lambda-initialization : service_common.go : env can not be empty INTERNAL_MESSAGES_FUNCTION_NAME")
	}
	Anlogger.Debugf(nil, "lambda-initialization : service_common.go : start with INTERNAL_MESSAGES_FUNCTION_NAME = [%s]", MessagesFunctionName)

	LMHISFunctionName, ok = os.LookupEnv("INTERNAL_LMHIS_FUNCTION_NAME")
	if !ok {
		Anlogger.Fatalf(nil, "lambda-initialization : service_common.go : env can not be empty INTERNAL_LMHIS_FUNCTION_NAME")
	}
	Anlogger.Debugf(nil, "lambda-initialization : service_common.go : start with INTERNAL_LMHIS_FUNCTION_NAME = [%s]", LMHISFunctionName)

	CommonStreamName, ok = os.LookupEnv("COMMON_STREAM")
	if !ok {
		Anlogger.Fatalf(nil, "lambda-initialization : service_common.go : env can not be empty COMMON_STREAM")
	}
	Anlogger.Debugf(nil, "lambda-initialization : service_common.go : start with COMMON_STREAM = [%s]", CommonStreamName)

	DeliveryStreamName, ok = os.LookupEnv("DELIVERY_STREAM")
	if !ok {
		Anlogger.Fatalf(nil, "lambda-initialization : service_common.go : env can not be empty DELIVERY_STREAM")
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

	userIdStatusEnabledMap = make(map[string]bool)
	//Kirill
	userIdStatusEnabledMap["d0b285a7d39f07e528dfba085e07a6135ddde188"] = true
	userIdStatusEnabledMap["ea6fa85e8afcf574d50c59b1d6cd1f2217fb718c"] = true
	userIdStatusEnabledMap["b9094fec646aa6296d0d3b3238801f92af34083a"] = true
	userIdStatusEnabledMap["54047644372b264ee02a1ac4e47cc6d02fc517bd"] = true
	//Iam
	userIdStatusEnabledMap["471d36399d776b7e3c4e031bf5e25cc67b452378"] = true
	//Victor
	userIdStatusEnabledMap["c86a29c241f8a0dadf3cff31b4c831bbfe3f2633"] = true
	//Maxim
	userIdStatusEnabledMap["f966276704b50ec1d472e34bbd184d89082bcdfb"] = true
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
	for index := range resp.Messages {
		resp.Messages[index].DefaultSortingOrderPosition = index
	}
	Anlogger.Debugf(lc, "service_common.go : successfully mark llm resp by default sort for userId [%s]", userId)
	return resp
}

func MarkLMHISDefaultSort(userId string, resp *LMHISFeedResp, lc *lambdacontext.LambdaContext) *LMHISFeedResp {
	Anlogger.Debugf(lc, "service_common.go : mark lmhis resp by default sort for userId [%s]", userId)
	for index := range resp.LikesYou {
		resp.LikesYou[index].DefaultSortingOrderPosition = index
	}
	for index := range resp.Matches {
		resp.Matches[index].DefaultSortingOrderPosition = index
	}
	for index := range resp.Hellos {
		resp.Hellos[index].DefaultSortingOrderPosition = index
	}
	for index := range resp.Inbox {
		resp.Inbox[index].DefaultSortingOrderPosition = index
	}
	for index := range resp.Sent {
		resp.Sent[index].DefaultSortingOrderPosition = index
	}
	Anlogger.Debugf(lc, "service_common.go : successfully mark llm resp by default sort for userId [%s]", userId)
	return resp
}

//return lastOnlineText, lastOnlineFlag
func TransformLastOnlineTimeIntoStatusText(userId string, lastOnlineTime int64, sourceLocale string, lc *lambdacontext.LambdaContext) (string, string) {
	Anlogger.Debugf(lc, "service_common.go : transform lastOnlineTime [%v] to status texts for userId [%s]", lastOnlineTime, userId)
	var lastOnlineText, lastOnlineFlag string
	currTime := commons.UnixTimeInMillis()
	diff := currTime - lastOnlineTime

	Anlogger.Debugf(lc, "service_common.go : diff is [%v] for userId [%s]", diff, userId)

	if lastOnlineTime <= 0 {
		lastOnlineText = "unknown"
		lastOnlineFlag = "unknown"
	} else {
		if diff <= 900000 { //15 min
			lastOnlineText = "Online"
			sl := strings.ToLower(sourceLocale)
			if sl == "ru" || sl == "be" || sl == "ua" {
				lastOnlineText = "Онлайн"
			}
		} else if diff > 900000 && diff <= 3599999 { //15 min < 59.99 min
			localM := "m ago"
			sl := strings.ToLower(sourceLocale)
			if sl == "ru" || sl == "be" || sl == "ua" {
				localM = "мин назад"
			}
			lastOnlineText = fmt.Sprintf("%v%s", diff/60000, localM)
		} else if diff >= 3600000 && diff <= 86400000 { // 1h < 24h
			localH := "h ago"
			sl := strings.ToLower(sourceLocale)
			if sl == "ru" || sl == "be" || sl == "ua" {
				localH = "ч назад"
			}
			lastOnlineText = fmt.Sprintf("%v%s", diff/3600000, localH)
		} else if diff > 86400000 && diff <= 172800000 { //24h < 48h
			lastOnlineText = "Yesterday"
			sl := strings.ToLower(sourceLocale)
			if sl == "ru" || sl == "be" || sl == "ua" {
				lastOnlineText = "Вчера"
			}
		} else if diff > 172800000 && diff <= 604800000 { //48h < 7 d
			localD := "d ago"
			sl := strings.ToLower(sourceLocale)
			if sl == "ru" || sl == "be" || sl == "ua" {
				localD = "д назад"
			}
			lastOnlineText = fmt.Sprintf("%v%s", diff/86400000, localD)
		} else {
			lastOnlineText = "unknown"
			lastOnlineFlag = "unknown"
			//lastOnlineText = "7+ days ago"
			//sl := strings.ToLower(sourceLocale)
			//if sl == "ru" || sl == "be" || sl == "ua" {
			//	lastOnlineText = "Больше недели назад"
			//}
		}

		if lastOnlineFlag != "unknown" {
			if diff <= 1800000 { //30 min
				lastOnlineFlag = "online"
			} else if diff > 1800000 && diff <= 10800000 {
				lastOnlineFlag = "away"
			} else {
				lastOnlineFlag = "offline"
			}
		}
	}

	Anlogger.Debugf(lc, "service_common.go : successfully transform lastOnlineTime [%v] to lastOnlineText [%s] and  lastOnlineFlag [%s] for userId [%s]", lastOnlineTime, lastOnlineText, lastOnlineFlag, userId)
	return lastOnlineText, lastOnlineFlag
}

//return distanceText
func TransformDistanceInDistanceText(userId string, internal commons.InternalProfiles, lc *lambdacontext.LambdaContext) (string) {
	Anlogger.Debugf(lc, "service_common.go : transform request [%v] to distance text for userId [%s]", internal, userId)
	var distanceText string
	if !internal.LocationExist {
		Anlogger.Debugf(lc, "service_common.go : one of the cordinates < 0, so set unknown distance text")
		distanceText = "unknown"
	} else {
		distance := Distance(Point(internal.Lat, internal.Lon), Point(internal.SourceLat, internal.SourceLon))
		Anlogger.Debugf(lc, "service_common.go : distance is [%v]", distance)
		if distance <= 1000 {
			distanceText = "1 "
		} else if distance > 1000 && distance <= 100000 {
			distanceText = fmt.Sprintf("%d", int(distance/1000))
		} else if distance > 100000 {
			distanceText = "100+ "
		} else {
			distanceText = "unknown"
		}
	}
	if distanceText != "unknown" {
		localKm := "km"
		sl := strings.ToLower(internal.SourceLocale)
		if sl == "ru" || sl == "be" || sl == "ua" {
			localKm = "км"
		}
		distanceText = fmt.Sprintf("%s%s", distanceText, localKm)
	}

	Anlogger.Debugf(lc, "service_common.go : successfully transform request [%v] to distance text [%s] for userId [%s]", internal, distanceText, userId)
	return distanceText
}
