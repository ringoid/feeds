package apimodel

import (
	"../sys_log"
	"github.com/aws/aws-sdk-go/aws"
	"encoding/json"
	"github.com/aws/aws-sdk-go/service/firehose"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/kinesis"
	"github.com/aws/aws-sdk-go/service/s3"
	"strings"
	"github.com/aws/aws-lambda-go/events"
	"time"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"strconv"
)

//return buildnum, is it android, ok and error string
func ParseAppVersionFromHeaders(headers map[string]string, anlogger *syslog.Logger, lc *lambdacontext.LambdaContext) (int, bool, bool, string) {
	anlogger.Debugf(lc, "common_action.go : parse build num from the headers %v", headers)
	var appVersionInt int
	var err error

	if appVersionStr, ok := headers[AndroidBuildNum]; ok {
		appVersionInt, err = strconv.Atoi(appVersionStr)
		if err != nil {
			anlogger.Errorf(lc, "common_action.go : error converting header [%s] with value [%s] to int : %v", AndroidBuildNum, appVersionStr, err)
			return 0, false, false, WrongRequestParamsClientError
		}
		anlogger.Debugf(lc, "common_action.go : successfully parse Android build num [%d] from the headers %v", appVersionInt, headers)
		return appVersionInt, true, true, ""

	} else if appVersionStr, ok = headers[iOSdBuildNum]; ok {
		appVersionInt, err = strconv.Atoi(appVersionStr)
		if err != nil {
			anlogger.Errorf(lc, "common_action.go : error converting header [%s] with value [%s] to int : %v", iOSdBuildNum, appVersionStr, err)
			return 0, false, false, WrongRequestParamsClientError
		}
		anlogger.Debugf(lc, "common_action.go : successfully parse iOS build num [%d] from the headers %v", appVersionInt, headers)
		return appVersionInt, false, true, ""
	} else {
		anlogger.Errorf(lc, "common_action.go : error header [%s] is empty", AndroidBuildNum)
		return 0, false, false, WrongRequestParamsClientError
	}
}

//return userId, ok, was user reported, error string
func CallVerifyAccessToken(buildNum int, isItAndroid bool, accessToken, functionName string, clientLambda *lambda.Lambda, anlogger *syslog.Logger, lc *lambdacontext.LambdaContext) (string, bool, bool, string) {
	req := InternalGetUserIdReq{
		AccessToken: accessToken,
		BuildNum:    buildNum,
		IsItAndroid: isItAndroid,
	}
	jsonBody, err := json.Marshal(req)
	if err != nil {
		anlogger.Errorf(lc, "common_action.go : error marshaling req %s into json : %v", req, err)
		return "", false, false, InternalServerError
	}

	resp, err := clientLambda.Invoke(&lambda.InvokeInput{FunctionName: aws.String(functionName), Payload: jsonBody})
	if err != nil {
		anlogger.Errorf(lc, "common_action.go : error invoke function [%s] with body %s : %v", functionName, jsonBody, err)
		return "", false, false, InternalServerError
	}

	if *resp.StatusCode != 200 {
		anlogger.Errorf(lc, "common_action.go : status code = %d, response body %s for request %s", *resp.StatusCode, string(resp.Payload), jsonBody)
		return "", false, false, InternalServerError
	}

	var response InternalGetUserIdResp
	err = json.Unmarshal(resp.Payload, &response)
	if err != nil {
		anlogger.Errorf(lc, "common_action.go : error unmarshaling response %s into json : %v", string(resp.Payload), err)
		return "", false, false, InternalServerError
	}

	if response.ErrorCode != "" {
		anlogger.Errorf(lc, "common_action.go : error response from function [%s], response=%v", functionName, response)
		switch response.ErrorCode {
		case "InvalidAccessTokenClientError":
			return "", false, false, InvalidAccessTokenClientError
		case "TooOldAppVersionClientError":
			return "", false, false, TooOldAppVersionClientError
		default:
			return "", false, false, InternalServerError
		}
	}

	anlogger.Debugf(lc, "common_action.go : successfully validate accessToken, userId [%s]", response.UserId)
	return response.UserId, true, response.IsUserReported, ""
}

func SendAnalyticEvent(event interface{}, userId, deliveryStreamName string, awsDeliveryStreamClient *firehose.Firehose,
	anlogger *syslog.Logger, lc *lambdacontext.LambdaContext) {
	anlogger.Debugf(lc, "common_action.go : send analytics event [%v] for userId [%s]", event, userId)
	data, err := json.Marshal(event)
	if err != nil {
		anlogger.Errorf(lc, "common_action.go : error marshaling analytics event [%v] for userId [%s] : %v", event, userId, err)
		return
	}
	newLine := "\n"
	data = append(data, newLine...)
	_, err = awsDeliveryStreamClient.PutRecord(&firehose.PutRecordInput{
		DeliveryStreamName: aws.String(deliveryStreamName),
		Record: &firehose.Record{
			Data: data,
		},
	})

	if err != nil {
		anlogger.Errorf(lc, "common_action.go : error sending analytics event [%v] for userId [%s] : %v", event, userId, err)
	}

	anlogger.Debugf(lc, "common_action.go : successfully send analytics event [%v] for userId [%s]", event, userId)
}

//ok and error string
func SendCommonEvent(event interface{}, userId, commonStreamName, partitionKey string, awsKinesisClient *kinesis.Kinesis,
	anlogger *syslog.Logger, lc *lambdacontext.LambdaContext) (bool, string) {
	anlogger.Debugf(lc, "common_action.go : send common event [%v] for userId [%s]", event, userId)
	data, err := json.Marshal(event)
	if err != nil {
		anlogger.Errorf(lc, "common_action.go : error marshaling common event [%v] for userId [%s] : %v", event, userId, err)
		return false, InternalServerError
	}
	if len(partitionKey) == 0 {
		partitionKey = userId
	}
	input := &kinesis.PutRecordInput{
		StreamName:   aws.String(commonStreamName),
		PartitionKey: aws.String(partitionKey),
		Data:         []byte(data),
	}
	_, err = awsKinesisClient.PutRecord(input)
	if err != nil {
		anlogger.Errorf(lc, "common_action.go : error putting common event into stream, event [%v] for userId [%s] : %v", event, userId, err)
		return false, InternalServerError
	}
	anlogger.Debugf(lc, "common_action.go : successfully send common event [%v] for userId [%s]", event, userId)
	return true, ""
}

//return ok and error string
func SendAsyncTask(task interface{}, asyncTaskQueue, userId string, messageSecDelay int64,
	awsSqsClient *sqs.SQS, anlogger *syslog.Logger, lc *lambdacontext.LambdaContext) (bool, string) {
	anlogger.Debugf(lc, "common_action.go : send async task %v for userId [%s] with delay in sec [%v]", task, userId, messageSecDelay)
	body, err := json.Marshal(task)
	if err != nil {
		anlogger.Errorf(lc, "common_action.go : error marshal task %v for userId [%s] with delay in sec [%v] : %v", task, userId, messageSecDelay, err)
		return false, InternalServerError
	}
	input := &sqs.SendMessageInput{
		DelaySeconds: aws.Int64(messageSecDelay),
		QueueUrl:     aws.String(asyncTaskQueue),
		MessageBody:  aws.String(string(body)),
	}
	_, err = awsSqsClient.SendMessage(input)
	if err != nil {
		anlogger.Errorf(lc, "common_action.go : error sending async task %v to the queue for userId [%s] with delay in sec [%v] : %v", task, userId, messageSecDelay, err)
		return false, InternalServerError
	}
	anlogger.Debugf(lc, "common_action.go : successfully send async task %v for userId [%s] with delay in sec [%v]", task, userId, messageSecDelay)
	return true, ""
}

//return ok and error string
func DeleteFromS3(bucket, key, userId string, awsS3Client *s3.S3, lc *lambdacontext.LambdaContext, anlogger *syslog.Logger) (bool, string) {
	anlogger.Debugf(lc, "common_action.go : delete from s3 bucket [%s] with key [%s] for userId [%s]",
		bucket, key, userId)

	input := &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	_, err := awsS3Client.DeleteObject(input)
	if err != nil {
		anlogger.Errorf(lc, "common_action.go : error delete from s3 bucket [%s] with key [%s] for userId [%s] : %v",
			bucket, key, userId, err)
		return false, InternalServerError
	}

	anlogger.Debugf(lc, "common_action.go : successfully delete from s3 bucket [%s] with key [%s] for userId [%s]",
		bucket, key, userId)
	return true, ""
}

func WarmUpLambda(functionName string, clientLambda *lambda.Lambda, anlogger *syslog.Logger, lc *lambdacontext.LambdaContext) {
	anlogger.Debugf(lc, "common_action.go : warmup lambda [%s]", functionName)
	req := WarmUpRequest{
		WarmUpRequest: true,
	}
	jsonBody, err := json.Marshal(req)
	if err != nil {
		anlogger.Errorf(lc, "common_action.go : error marshaling req %v into json : %v", req, err)
		return
	}

	apiReq := events.APIGatewayProxyRequest{
		Body: string(jsonBody),
	}

	apiJsonBody, err := json.Marshal(apiReq)
	if err != nil {
		anlogger.Errorf(lc, "common_action.go : error marshaling req %v into json : %v", apiReq, err)
		return
	}

	payload := apiJsonBody
	if strings.Contains(functionName, "internal") {
		payload = jsonBody
	}
	_, err = clientLambda.Invoke(&lambda.InvokeInput{FunctionName: aws.String(functionName), Payload: payload, InvocationType: aws.String("Event")})

	if err != nil {
		anlogger.Errorf(lc, "warm_up.go : error invoke function [%s] with body %s : %v", functionName, string(payload), err)
		return
	}

	anlogger.Debugf(lc, "common_action.go : successfully warmup lambda [%s]", functionName)
	return
}

func IsItWarmUpRequest(body string, anlogger *syslog.Logger, lc *lambdacontext.LambdaContext) bool {
	anlogger.Debugf(lc, "common_action.go : is it warm up request, body [%s]", body)
	if len(body) == 0 {
		anlogger.Debugf(lc, "common_action.go : empty request body, it's no warm up request")
		return false
	}
	var req WarmUpRequest
	err := json.Unmarshal([]byte(body), &req)

	if err != nil {
		anlogger.Errorf(lc, "common_action.go : error unmarshal required params from the string [%s] : %v", body, err)
		return false
	}
	result := req.WarmUpRequest
	anlogger.Debugf(lc, "common_action.go : successfully check that it's warm up request, body [%s], result [%v]", body, result)
	return result
}

//return ok and error string
func SendCloudWatchMetric(baseCloudWatchNamespace, metricName string, value int, cwClient *cloudwatch.CloudWatch, anlogger *syslog.Logger, lc *lambdacontext.LambdaContext) (bool, string) {
	anlogger.Debugf(lc, "common_action.go : send value [%d] for namespace [%s] and metric name [%s]", value, baseCloudWatchNamespace, metricName)

	currentTime := time.Now().UTC()

	peD := cloudwatch.MetricDatum{
		MetricName: aws.String(metricName),
		Timestamp:  &currentTime,
		Value:      aws.Float64(float64(value)),
	}

	metricdatas := []*cloudwatch.MetricDatum{&peD}

	_, err := cwClient.PutMetricData(&cloudwatch.PutMetricDataInput{
		MetricData: metricdatas,
		Namespace:  aws.String(baseCloudWatchNamespace),
	})
	if err != nil {
		anlogger.Errorf(lc, "common_action.go : error sending cloudwatch metric with value [%d] for namespace [%s] and metric name [%s] : %v", value, baseCloudWatchNamespace, metricName, err)
		return false, InternalServerError
	}

	anlogger.Debugf(lc, "common_action.go : successfully send value [%d] for namespace [%s] and metric name [%s]", value, baseCloudWatchNamespace, metricName)
	return true, ""
}

func GetOriginPhotoId(userId, sourcePhotoId string, anlogger *syslog.Logger, lc *lambdacontext.LambdaContext) (string, bool) {
	anlogger.Debugf(lc, "common_action.go : get origin photo id based on source photo id [%s] for userId [%s]", sourcePhotoId, userId)
	if len(sourcePhotoId) == 0 {
		anlogger.Warnf(lc, "common_action.go : empty source photo id for userId [%s]", userId)
		return "", false
	}
	arr := strings.Split(sourcePhotoId, "_")
	if len(arr) != 2 {
		anlogger.Warnf(lc, "common_action.go : wrong source photo id for userId [%s]", userId)
		return "", false
	}
	baseId := arr[1]
	originPhotoId := "origin_" + baseId
	anlogger.Debugf(lc, "common_action.go : successfully get origin photo id [%s] for source photo id [%s] for userId [%s]",
		originPhotoId, sourcePhotoId, userId)
	return originPhotoId, true
}
