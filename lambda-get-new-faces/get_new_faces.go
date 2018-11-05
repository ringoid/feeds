package main

import (
	"context"
	basicLambda "github.com/aws/aws-lambda-go/lambda"
	"../sys_log"
	"../apimodel"
	"github.com/aws/aws-sdk-go/aws"
	"os"
	"fmt"
	"github.com/aws/aws-sdk-go/aws/session"
	"encoding/json"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go/service/lambda"
	"time"
	"github.com/aws/aws-sdk-go/service/kinesis"
	"strconv"
	"github.com/aws/aws-sdk-go/service/firehose"
)

var anlogger *syslog.Logger
var internalAuthFunctionName string
var getNewFacesFunctionName string
var clientLambda *lambda.Lambda
var commonStreamName string
var awsKinesisClient *kinesis.Kinesis
var deliveryStramName string
var awsDeliveryStreamClient *firehose.Firehose

const (
	newFacesDefaultLimit                   = 5
	newFacesMaxLimit                       = 100
	newFacesTimeToLiveLimitForViewRelInSec = 60 * 5
)

func init() {
	var env string
	var ok bool
	var papertrailAddress string
	var err error
	var awsSession *session.Session

	env, ok = os.LookupEnv("ENV")
	if !ok {
		fmt.Printf("lambda-initialization : get_new_faces.go : env can not be empty ENV\n")
		os.Exit(1)
	}
	fmt.Printf("lambda-initialization : get_new_faces.go : start with ENV = [%s]\n", env)

	papertrailAddress, ok = os.LookupEnv("PAPERTRAIL_LOG_ADDRESS")
	if !ok {
		fmt.Printf("lambda-initialization : get_new_faces.go : env can not be empty PAPERTRAIL_LOG_ADDRESS\n")
		os.Exit(1)
	}
	fmt.Printf("lambda-initialization : get_new_faces.go : start with PAPERTRAIL_LOG_ADDRESS = [%s]\n", papertrailAddress)

	anlogger, err = syslog.New(papertrailAddress, fmt.Sprintf("%s-%s", env, "get-new-faces-get_new_faces"))
	if err != nil {
		fmt.Errorf("lambda-initialization : get_new_faces.go : error during startup : %v\n", err)
		os.Exit(1)
	}
	anlogger.Debugf(nil, "lambda-initialization : get_new_faces.go : logger was successfully initialized")

	internalAuthFunctionName, ok = os.LookupEnv("INTERNAL_AUTH_FUNCTION_NAME")
	if !ok {
		anlogger.Fatalf(nil, "lambda-initialization : get_new_faces.go : env can not be empty INTERNAL_AUTH_FUNCTION_NAME")
	}
	anlogger.Debugf(nil, "lambda-initialization : get_new_faces.go : start with INTERNAL_AUTH_FUNCTION_NAME = [%s]", internalAuthFunctionName)

	getNewFacesFunctionName, ok = os.LookupEnv("INTERNAL_GET_NEW_FACES_FUNCTION_NAME")
	if !ok {
		anlogger.Fatalf(nil, "lambda-initialization : get_new_faces.go : env can not be empty INTERNAL_GET_NEW_FACES_FUNCTION_NAME")
	}
	anlogger.Debugf(nil, "lambda-initialization : get_new_faces.go : start with INTERNAL_GET_NEW_FACES_FUNCTION_NAME = [%s]", getNewFacesFunctionName)

	commonStreamName, ok = os.LookupEnv("COMMON_STREAM")
	if !ok {
		anlogger.Fatalf(nil, "lambda-initialization : get_new_faces.go : env can not be empty COMMON_STREAM")
		os.Exit(1)
	}
	anlogger.Debugf(nil, "lambda-initialization : get_new_faces.go : start with COMMON_STREAM = [%s]", commonStreamName)

	deliveryStramName, ok = os.LookupEnv("DELIVERY_STREAM")
	if !ok {
		anlogger.Fatalf(nil, "lambda-initialization : get_new_faces.go : env can not be empty DELIVERY_STREAM")
		os.Exit(1)
	}
	anlogger.Debugf(nil, "lambda-initialization : get_new_faces.go : start with DELIVERY_STREAM = [%s]", deliveryStramName)

	awsSession, err = session.NewSession(aws.NewConfig().
		WithRegion(apimodel.Region).WithMaxRetries(apimodel.MaxRetries).
		WithLogger(aws.LoggerFunc(func(args ...interface{}) { anlogger.AwsLog(args) })).WithLogLevel(aws.LogOff))
	if err != nil {
		anlogger.Fatalf(nil, "lambda-initialization : get_new_faces.go : error during initialization : %v", err)
	}
	anlogger.Debugf(nil, "lambda-initialization : get_new_faces.go : aws session was successfully initialized")

	clientLambda = lambda.New(awsSession)
	anlogger.Debugf(nil, "lambda-initialization : get_new_faces.go : lambda client was successfully initialized")

	awsKinesisClient = kinesis.New(awsSession)
	anlogger.Debugf(nil, "lambda-initialization : get_new_faces.go : kinesis client was successfully initialized")

	awsDeliveryStreamClient = firehose.New(awsSession)
	anlogger.Debugf(nil, "lambda-initialization : get_new_faces.go : firehose client was successfully initialized")

}

func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	lc, _ := lambdacontext.FromContext(ctx)

	anlogger.Debugf(lc, "get_new_faces.go : start handle request %v", request)

	if apimodel.IsItWarmUpRequest(request.Body, anlogger, lc) {
		return events.APIGatewayProxyResponse{}, nil
	}

	appVersion, isItAndroid, ok, errStr := apimodel.ParseAppVersionFromHeaders(request.Headers, anlogger, lc)
	if !ok {
		anlogger.Errorf(lc, "get_new_faces.go : return %s to client", errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
	}

	accessToken := request.QueryStringParameters["accessToken"]
	resolution := request.QueryStringParameters["resolution"]
	limit := newFacesDefaultLimit
	limitStr := request.QueryStringParameters["limit"]
	var err error
	if len(limitStr) != 0 {
		limit, err = strconv.Atoi(limitStr)
		if err != nil {
			errStr = apimodel.WrongRequestParamsClientError
			anlogger.Errorf(lc, "get_new_faces.go : return %s to client", errStr)
			return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
		}
	}

	if !apimodel.AllowedPhotoResolution[resolution] {
		errStr := apimodel.WrongRequestParamsClientError
		anlogger.Errorf(lc, "get_new_faces : resolution [%s] is not supported", resolution)
		anlogger.Errorf(lc, "get_new_faces.go : return %s to client", errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
	}

	userId, ok, _, errStr := apimodel.CallVerifyAccessToken(appVersion, isItAndroid, accessToken, internalAuthFunctionName, clientLambda, anlogger, lc)
	if !ok {
		anlogger.Errorf(lc, "get_new_faces.go : return %s to client", errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
	}

	internalNewFaces, ok, errStr := getNewFaces(userId, limit, lc)
	if !ok {
		anlogger.Errorf(lc, "get_new_faces.go : return %s to client", errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
	}

	targetIds := make([]string, 0)

	profiles := make([]apimodel.Profile, 0)
	for _, each := range internalNewFaces {
		photos := make([]apimodel.Photo, 0)
		for _, eachPhoto := range each.PhotoIds {
			photos = append(photos, apimodel.Photo{
				PhotoId: eachPhoto,
				//todo:implement real photo uri
				PhotoUri: "test-photo-uri",
			})
		}
		profiles = append(profiles, apimodel.Profile{
			UserId: each.UserId,
			Photos: photos,
		})

		targetIds = append(targetIds, each.UserId)
	}
	anlogger.Debugf(lc, "get_new_faces.go : prepare [%d] new faces profiles for userId [%s]", len(profiles), userId)

	timeToDeleteViewRel := time.Now().Unix() + newFacesTimeToLiveLimitForViewRelInSec
	event := apimodel.NewProfileWasReturnToNewFacesEvent(userId, timeToDeleteViewRel, targetIds)
	ok, errStr = apimodel.SendCommonEvent(event, userId, commonStreamName, userId, awsKinesisClient, anlogger, lc)
	if !ok {
		errStr := apimodel.InternalServerError
		anlogger.Errorf(lc, "get_new_faces.go : userId [%s], return %s to client", userId, errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
	}

	apimodel.SendAnalyticEvent(event, userId, deliveryStramName, awsDeliveryStreamClient, anlogger, lc)

	resp := apimodel.GetNewFacesResp{}
	resp.Profiles = profiles
	body, err := json.Marshal(resp)
	if err != nil {
		anlogger.Errorf(lc, "get_new_faces.go : error while marshaling resp [%v] object for userId [%s] : %v", resp, userId, err)
		anlogger.Errorf(lc, "get_new_faces.go : userId [%s], return %s to client", userId, apimodel.InternalServerError)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: apimodel.InternalServerError}, nil
	}
	anlogger.Debugf(lc, "get_new_faces.go : return successful resp [%s] for userId [%s]", string(body), userId)
	return events.APIGatewayProxyResponse{StatusCode: 200, Body: string(body)}, nil
}

func getNewFaces(userId string, limit int, lc *lambdacontext.LambdaContext) ([]apimodel.InternalNewFace, bool, string) {

	if limit < 0 {
		limit = newFacesDefaultLimit
	} else if limit > newFacesMaxLimit {
		limit = newFacesMaxLimit
	}
	anlogger.Debugf(lc, "get_new_faces.go : get new faces for userId [%s] with limit [%d]", userId, limit)

	req := apimodel.InternalGetNewFacesReq{
		UserId:
		userId,
		Limit: limit,
	}
	jsonBody, err := json.Marshal(req)
	if err != nil {
		anlogger.Errorf(lc, "get_new_faces.go : error marshaling req %s into json : %v", req, err)
		return nil, false, apimodel.InternalServerError
	}

	resp, err := clientLambda.Invoke(&lambda.InvokeInput{FunctionName: aws.String(getNewFacesFunctionName), Payload: jsonBody})
	if err != nil {
		anlogger.Errorf(lc, "get_new_faces.go : error invoke function [%s] with body %s : %v", getNewFacesFunctionName, jsonBody, err)
		return nil, false, apimodel.InternalServerError
	}

	if *resp.StatusCode != 200 {
		anlogger.Errorf(lc, "get_new_faces.go : status code = %d, response body %s for request %s", *resp.StatusCode, string(resp.Payload), jsonBody)
		return nil, false, apimodel.InternalServerError
	}

	var response apimodel.InternalGetNewFacesResp
	err = json.Unmarshal(resp.Payload, &response)
	if err != nil {
		anlogger.Errorf(lc, "get_new_faces.go : error unmarshaling response %s into json : %v", string(resp.Payload), err)
		return nil, false, apimodel.InternalServerError
	}

	anlogger.Debugf(lc, "get_new_faces.go : successfully got new faces for userId [%s] with limit [%d], resp %v", userId, limit, response)
	return response.NewFaces, true, ""
}

func main() {
	basicLambda.Start(handler)
}
