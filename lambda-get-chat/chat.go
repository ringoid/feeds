package main

import (
	"context"
	basicLambda "github.com/aws/aws-lambda-go/lambda"
	"../apimodel"
	"github.com/aws/aws-sdk-go/aws"
	"encoding/json"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go/service/lambda"
	"strconv"
	"github.com/ringoid/commons"
	"strings"
)

func init() {
	apimodel.InitLambdaVars("chat-feed")
}

func handler(ctx context.Context, request events.ALBTargetGroupRequest) (events.ALBTargetGroupResponse, error) {
	startTime := commons.UnixTimeInMillis()

	lc, _ := lambdacontext.FromContext(ctx)

	userAgent := request.Headers["user-agent"]
	if strings.HasPrefix(userAgent, "ELB-HealthChecker") {
		return commons.NewServiceResponse("{}"), nil
	}

	if request.HTTPMethod != "GET" {
		return commons.NewWrongHttpMethodServiceResponse(), nil
	}
	sourceIp := request.Headers["x-forwarded-for"]

	apimodel.Anlogger.Debugf(lc, "chat.go : start handle request %v", request)

	appVersion, isItAndroid, ok, errStr := commons.ParseAppVersionFromHeaders(request.Headers, apimodel.Anlogger, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "chat.go : return %s to client", errStr)
		return commons.NewServiceResponse(errStr), nil
	}

	accessToken, okA := request.QueryStringParameters["accessToken"]
	resolution, okR := request.QueryStringParameters["resolution"]
	lastActionTimeStr, okL := request.QueryStringParameters["lastActionTime"]
	oppositeUserId, okU := request.QueryStringParameters["userId"]

	if !okA || !okR || !okL || !okU {
		errStr = commons.WrongRequestParamsClientError
		apimodel.Anlogger.Errorf(lc, "chat.go : okA [%v], okR [%v], okL [%v] and okU [%v]", okA, okR, okL, okU)
		apimodel.Anlogger.Errorf(lc, "chat.go : return %s to client", errStr)
		return commons.NewServiceResponse(errStr), nil
	}

	if !commons.AllowedPhotoResolution[resolution] {
		apimodel.Anlogger.Warnf(lc, "chat.go : resolution [%s] is not supported, so use [%s] resolution", resolution, commons.BiggestDefaultPhotoResolution)
		resolution = commons.BiggestDefaultPhotoResolution
	}

	lastActionTimeInt64, err := strconv.ParseInt(lastActionTimeStr, 10, 64)
	if err != nil || lastActionTimeInt64 < 0 {
		errStr := commons.WrongRequestParamsClientError
		apimodel.Anlogger.Errorf(lc, "chat.go : lastActionTime in wrong format [%s]", lastActionTimeStr)
		apimodel.Anlogger.Errorf(lc, "chat.go : return %s to client", errStr)
		return commons.NewServiceResponse(errStr), nil
	}

	userId, ok, _, errStr := commons.CallVerifyAccessToken(appVersion, isItAndroid, accessToken, apimodel.InternalAuthFunctionName, apimodel.ClientLambda, apimodel.Anlogger, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "chat.go : return %s to client", errStr)
		return commons.NewServiceResponse(errStr), nil
	}

	internalChat, repeatRequestAfter, ok, errStr := getChat(userId, oppositeUserId, lastActionTimeInt64, resolution, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "chat.go : userId [%s], return %s to client", userId, errStr)
		return commons.NewServiceResponse(errStr), nil
	}

	feedResp := apimodel.ChatFeedResponse{}

	feedResp.RepeatRequestAfter = repeatRequestAfter
	feedResp.IsChatExists = internalChat.IsChatExists
	photos := make([]commons.Photo, 0)
	for _, eachPhoto := range internalChat.Profile.Photos {
		photos = append(photos, commons.Photo{
			PhotoId:           eachPhoto.ResizedPhotoId,
			PhotoUri:          eachPhoto.Link,
			ThumbnailPhotoUri: eachPhoto.ThumbnailLink,
		})
	}
	if len(photos) == 0 {
		apimodel.Anlogger.Warnf(lc, "chat.go : get chat return user [%s] with empty photo list for resolution [%s] for userId [%s]",
			oppositeUserId, resolution, userId)
	}

	msgs := make([]commons.Message, 0)
	for _, eachMsg := range internalChat.Profile.Messages {
		msgs = append(msgs, eachMsg)
	}

	lastOnlineText, lastOnlineFlag := apimodel.TransformLastOnlineTimeIntoStatusText(userId, internalChat.Profile.LastOnlineTime, internalChat.Profile.SourceLocale, lc)
	distanceText := apimodel.TransformDistanceInDistanceText(userId, internalChat.Profile, lc)
	profile := commons.Profile{
		UserId:         internalChat.Profile.UserId,
		Photos:         photos,
		Messages:       msgs,
		LastOnlineText: lastOnlineText,
		LastOnlineFlag: lastOnlineFlag,
		DistanceText:   distanceText,
		Age:            internalChat.Profile.Age,
		Property:       internalChat.Profile.Property,
		Transport:      internalChat.Profile.Transport,
		Income:         internalChat.Profile.Income,
		Height:         internalChat.Profile.Height,
		EducationLevel: internalChat.Profile.EducationLevel,
		HairColor:      internalChat.Profile.HairColor,
	}

	profile = apimodel.CheckProfileBeforeResponse(userId, profile)

	feedResp.ProfileChat = profile
	feedResp.PullAgainAfter = apimodel.DefaultPoolRepeatTimeSec

	body, err := json.Marshal(feedResp)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "chat.go : error while marshaling resp [%v] object for userId [%s] : %v", feedResp, userId, err)
		apimodel.Anlogger.Errorf(lc, "chat.go : userId [%s], return %s to client", userId, commons.InternalServerError)
		return commons.NewServiceResponse(commons.InternalServerError), nil
	}

	event := commons.NewChatWasReturnEvent(userId, sourceIp, oppositeUserId, len(feedResp.ProfileChat.Messages), feedResp.RepeatRequestAfter, feedResp.PullAgainAfter)
	commons.SendAnalyticEvent(event, userId, apimodel.DeliveryStreamName, apimodel.AwsDeliveryStreamClient, apimodel.Anlogger, lc)
	//commons.SendCloudWatchMetric(apimodel.BaseCloudWatchNamespace, apimodel.NewFaceProfilesReturnMetricName, len(feedResp.Profiles), apimodel.AwsCWClient, apimodel.Anlogger, lc)
	finishTime := commons.UnixTimeInMillis()
	apimodel.Anlogger.Infof(lc, "chat.go : successfully return repeat request after [%v], chat to userId [%s] with oppositeUserId [%s], duration [%v]", feedResp.RepeatRequestAfter, userId, feedResp.ProfileChat.UserId, finishTime-startTime)
	apimodel.Anlogger.Debugf(lc, "chat.go : return successful resp [%s] for userId [%s]", string(body), userId)
	return commons.NewServiceResponse(string(body)), nil
}

//response, repeat request after sec, ok and error string
func getChat(userId, oppositeUserId string, lastActionTime int64, resolution string, lc *lambdacontext.LambdaContext) (commons.InternalChatResponse, int64, bool, string) {

	apimodel.Anlogger.Debugf(lc, "chat.go : get chat userId [%s] and oppositeUserId [%s]", userId, oppositeUserId)

	req := commons.InternalChatRequest{
		UserId:         userId,
		OppositeUserId: oppositeUserId,
		LastActionTime: lastActionTime,
		Resolution:     resolution,
	}
	jsonBody, err := json.Marshal(req)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "chat.go : error marshaling req %s into json for userId [%s] : %v", req, userId, err)
		return commons.InternalChatResponse{}, 0, false, commons.InternalServerError
	}

	resp, err := apimodel.ClientLambda.Invoke(&lambda.InvokeInput{FunctionName: aws.String(apimodel.ChatFunctionName), Payload: jsonBody})
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "chat.go : error invoke function [%s] with body %s for userId [%s] : %v", apimodel.ChatFunctionName, jsonBody, userId, err)
		return commons.InternalChatResponse{}, 0, false, commons.InternalServerError
	}

	if *resp.StatusCode != 200 {
		apimodel.Anlogger.Errorf(lc, "chat.go : status code = %d, response body %s for request %s, for userId [%s] ", *resp.StatusCode, string(resp.Payload), jsonBody, userId)
		return commons.InternalChatResponse{}, 0, false, commons.InternalServerError
	}

	var response commons.InternalChatResponse
	err = json.Unmarshal(resp.Payload, &response)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "chat.go : error unmarshaling response %s into json for userId [%s] : %v", string(resp.Payload), userId, err)
		return commons.InternalChatResponse{}, 0, false, commons.InternalServerError
	}

	if lastActionTime > response.LastActionTime {
		apimodel.Anlogger.Debugf(lc, "chat.go : requested lastActionTime [%d] > actual lastActionTime [%d] for userId [%s], diff is [%d]",
			lastActionTime, response.LastActionTime, userId, response.LastActionTime-lastActionTime)
		return commons.InternalChatResponse{}, apimodel.DefaultRepeatTimeSec, true, ""
	}

	apimodel.Anlogger.Debugf(lc, "chat.go : successfully got chat for userId [%s] and oppositeUserId [%s], resp %v", userId, oppositeUserId, response)
	return response, 0, true, ""
}

func main() {
	basicLambda.Start(handler)
}
