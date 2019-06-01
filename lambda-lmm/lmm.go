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
	"github.com/ringoid/commons"
	"strconv"
	"sync"
	"strings"
)

func init() {
	apimodel.InitLambdaVars("lmm-feed")
}

func handleJob(userId, resolution string, lastActionTimeInt int64, requestNewPart bool, functionName string, innerResult *InnerLmmResult,
	wg *sync.WaitGroup, lc *lambdacontext.LambdaContext) {
	defer wg.Done()

	llmResult, ok, errStr := llm(userId, functionName, requestNewPart, lastActionTimeInt, resolution, lc)
	if !ok {
		innerResult.ok = ok
		innerResult.errStr = errStr
		return
	}

	if lastActionTimeInt > llmResult.LastActionTime {
		innerResult.ok = true
		innerResult.repeatRequestAfter = apimodel.DefaultRepeatTimeSec
		apimodel.Anlogger.Debugf(lc, "lmm.go : (%s) requested lastAction time [%v] > actual last actionTime [%v], diff [%v]",
			functionName, lastActionTimeInt, llmResult.LastActionTime, llmResult.LastActionTime-lastActionTimeInt)
		return
	}

	profiles := make([]commons.Profile, 0)
	for _, each := range llmResult.Profiles {

		photos := make([]commons.Photo, 0)

		for _, eachPhoto := range each.Photos {
			photos = append(photos, commons.Photo{
				PhotoId:           eachPhoto.ResizedPhotoId,
				PhotoUri:          eachPhoto.Link,
				ThumbnailPhotoUri: eachPhoto.ThumbnailLink,
			})
		}

		messages := make([]commons.Message, 0)
		for _, eachMessage := range each.Messages {
			messages = append(messages, eachMessage)
		}

		if len(photos) == 0 {
			apimodel.Anlogger.Warnf(lc, "lmm.go : lmm return user [%s] with empty photo list for resolution [%s] for userId [%s]",
				each.UserId, resolution, userId)
			continue
		}

		lastOnlineText, lastOnlineFlag := apimodel.TransformLastOnlineTimeIntoStatusText(userId, each.LastOnlineTime, each.SourceLocale, lc)
		distanceText := apimodel.TransformDistanceInDistanceText(userId, each, lc)

		profile := commons.Profile{
			UserId:         each.UserId,
			Photos:         photos,
			Unseen:         requestNewPart,
			Messages:       messages,
			LastOnlineText: lastOnlineText,
			LastOnlineFlag: lastOnlineFlag,
			DistanceText:   distanceText,
			Age:            each.Age,
			Sex:            each.Sex,
			Property:       each.Property,
			Transport:      each.Transport,
			Income:         each.Income,
			Height:         each.Height,
			EducationLevel: each.EducationLevel,
			HairColor:      each.HairColor,
		}

		profile = apimodel.CheckProfileBeforeResponse(userId, profile)

		profiles = append(profiles, profile)
	}
	apimodel.Anlogger.Debugf(lc, "lmm.go : prepare [%d] likes you profiles for userId [%s]", len(profiles), userId)

	innerResult.ok = true
	innerResult.profiles = profiles

	return
}

type InnerLmmResult struct {
	ok                 bool
	errStr             string
	repeatRequestAfter int64
	profiles           []commons.Profile
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

	apimodel.Anlogger.Debugf(lc, "lmm.go : start handle request %v", request)

	appVersion, isItAndroid, ok, errStr := commons.ParseAppVersionFromHeaders(request.Headers, apimodel.Anlogger, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "lmm.go : return %s to client", errStr)
		return commons.NewServiceResponse(errStr), nil
	}

	accessToken, okA := request.QueryStringParameters["accessToken"]
	resolution, okR := request.QueryStringParameters["resolution"]
	lastActionTimeStr, okL := request.QueryStringParameters["lastActionTime"]

	source, okS := request.QueryStringParameters["source"]
	if okS {
		if _, ok := commons.FeedNames[source]; !ok && source != "profile" {
			errStr = commons.WrongRequestParamsClientError
			apimodel.Anlogger.Errorf(lc, "lmm.go : source contains unsupported value [%s]", source)
			return commons.NewServiceResponse(errStr), nil
		}
	}

	if !okA || !okR || !okL {
		errStr = commons.WrongRequestParamsClientError
		apimodel.Anlogger.Errorf(lc, "lmm.go : okA [%v], okR [%v] and okL [%v]", okA, okR, okL)
		apimodel.Anlogger.Errorf(lc, "lmm.go : return %s to client", errStr)
		return commons.NewServiceResponse(errStr), nil
	}

	if !commons.AllowedPhotoResolution[resolution] {
		apimodel.Anlogger.Warnf(lc, "lmm.go : resolution [%s] is not supported, so use [%s] resolution", resolution, commons.BiggestDefaultPhotoResolution)
		resolution = commons.BiggestDefaultPhotoResolution
	}

	lastActionTimeInt64, err := strconv.ParseInt(lastActionTimeStr, 10, 64)
	if err != nil || lastActionTimeInt64 < 0 {
		errStr := commons.WrongRequestParamsClientError
		apimodel.Anlogger.Errorf(lc, "lmm.go : lastActionTime in wrong format [%s]", lastActionTimeStr)
		apimodel.Anlogger.Errorf(lc, "lmm.go : return %s to client", errStr)
		return commons.NewServiceResponse(errStr), nil
	}

	userId, ok, _, errStr := commons.CallVerifyAccessToken(appVersion, isItAndroid, accessToken, apimodel.InternalAuthFunctionName, apimodel.ClientLambda, apimodel.Anlogger, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "lmm.go : return %s to client", errStr)
		return commons.NewServiceResponse(errStr), nil
	}

	//prepare response
	feedResp := apimodel.LMMFeedResp{}
	feedResp.LikesYou = make([]commons.Profile, 0)
	feedResp.Matches = make([]commons.Profile, 0)
	feedResp.Messages = make([]commons.Profile, 0)

	var commonWaitGroup sync.WaitGroup

	//likes you (new part)
	commonWaitGroup.Add(1)
	likesYouNewPart := InnerLmmResult{}
	go handleJob(userId, resolution, lastActionTimeInt64, true, apimodel.LikesYouFunctionName, &likesYouNewPart,
		&commonWaitGroup, lc)

	//likes you (old part)
	commonWaitGroup.Add(1)
	likesYouOldPart := InnerLmmResult{}
	go handleJob(userId, resolution, lastActionTimeInt64, false, apimodel.LikesYouFunctionName, &likesYouOldPart,
		&commonWaitGroup, lc)

	//matches (new part)
	commonWaitGroup.Add(1)
	matchesNewPart := InnerLmmResult{}
	go handleJob(userId, resolution, lastActionTimeInt64, true, apimodel.MatchesFunctionName, &matchesNewPart,
		&commonWaitGroup, lc)

	//matches (old part)
	commonWaitGroup.Add(1)
	matchesOldPart := InnerLmmResult{}
	go handleJob(userId, resolution, lastActionTimeInt64, false, apimodel.MatchesFunctionName, &matchesOldPart,
		&commonWaitGroup, lc)

	//messages
	commonWaitGroup.Add(1)
	messagesPart := InnerLmmResult{}
	go handleJob(userId, resolution, lastActionTimeInt64, false, apimodel.MessagesFunctionName, &messagesPart,
		&commonWaitGroup, lc)

	commonWaitGroup.Wait()

	if !likesYouNewPart.ok || !likesYouOldPart.ok ||
		!matchesNewPart.ok || !matchesOldPart.ok ||
		!messagesPart.ok {
		apimodel.Anlogger.Errorf(lc, "lmm.go : userId [%s], return %s to client", userId, likesYouNewPart.errStr)
		return commons.NewServiceResponse(likesYouNewPart.errStr), nil
	}

	if likesYouNewPart.repeatRequestAfter != 0 || likesYouOldPart.repeatRequestAfter != 0 ||
		matchesNewPart.repeatRequestAfter != 0 || matchesOldPart.repeatRequestAfter != 0 ||
		messagesPart.repeatRequestAfter != 0 {
		apimodel.Anlogger.Debugf(lc, "lmm.go : return repeat request after [%v] for userId [%s]", apimodel.DefaultRepeatTimeSec, userId)
		feedResp.RepeatRequestAfter = apimodel.DefaultRepeatTimeSec
	} else {
		feedResp.LikesYou = append(feedResp.LikesYou, likesYouNewPart.profiles...)
		feedResp.LikesYou = append(feedResp.LikesYou, likesYouOldPart.profiles...)

		feedResp.Matches = append(feedResp.Matches, matchesNewPart.profiles...)
		feedResp.Matches = append(feedResp.Matches, matchesOldPart.profiles...)

		feedResp.Messages = append(feedResp.Messages, messagesPart.profiles...)
	}

	//mark sorting
	apimodel.MarkLMMDefaultSort(userId, &feedResp, lc)

	body, err := json.Marshal(feedResp)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "lmm.go : error while marshaling resp [%v] object for userId [%s] : %v", feedResp, userId, err)
		apimodel.Anlogger.Errorf(lc, "lmm.go : userId [%s], return %s to client", userId, commons.InternalServerError)
		return commons.NewServiceResponse(commons.InternalServerError), nil
	}

	event := commons.NewProfileWasReturnToLMMEvent(userId, sourceIp, source, len(feedResp.LikesYou), len(feedResp.Matches), len(feedResp.Messages), feedResp.RepeatRequestAfter)
	commons.SendAnalyticEvent(event, userId, apimodel.DeliveryStreamName, apimodel.AwsDeliveryStreamClient, apimodel.Anlogger, lc)

	//commons.SendCloudWatchMetric(apimodel.BaseCloudWatchNamespace, apimodel.LikesYouProfilesReturnMetricName, len(feedResp.LikesYou), apimodel.AwsCWClient, apimodel.Anlogger, lc)
	//commons.SendCloudWatchMetric(apimodel.BaseCloudWatchNamespace, apimodel.MatchProfilesReturnMetricName, len(feedResp.Matches), apimodel.AwsCWClient, apimodel.Anlogger, lc)
	//commons.SendCloudWatchMetric(apimodel.BaseCloudWatchNamespace, apimodel.MessageProfilesReturnMetricName, len(feedResp.Messages), apimodel.AwsCWClient, apimodel.Anlogger, lc)

	finishTime := commons.UnixTimeInMillis()
	apimodel.Anlogger.Infof(lc, "lmm.go : successfully return repeat request after [%v], [%d] likes you profiles, [%d] matches and [%d] messages to userId [%s], duration [%v]", feedResp.RepeatRequestAfter, len(feedResp.LikesYou), len(feedResp.Matches), len(feedResp.Messages), userId, finishTime-startTime)
	apimodel.Anlogger.Debugf(lc, "lmm.go : return successful resp [%s] for userId [%s]", string(body), userId)
	return commons.NewServiceResponse(string(body)), nil
}

func llm(userId, functionName string, requestNewPart bool, lastActionTime int64, resolution string, lc *lambdacontext.LambdaContext) (commons.InternalLMMResp, bool, string) {

	apimodel.Anlogger.Debugf(lc, "lmm.go : get llm (function name %s, request new part %v) you for userId [%s]", functionName, requestNewPart, userId)

	req := commons.InternalLMMReq{
		UserId:                  userId,
		RequestNewPart:          requestNewPart,
		RequestedLastActionTime: lastActionTime,
		Resolution:              resolution,
	}
	jsonBody, err := json.Marshal(req)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "lmm.go : error marshaling req %s into json for userId [%s] (function name %s, request new part %v) : %v",
			req, userId, functionName, requestNewPart, err)
		return commons.InternalLMMResp{}, false, commons.InternalServerError
	}

	resp, err := apimodel.ClientLambda.Invoke(&lambda.InvokeInput{FunctionName: aws.String(functionName), Payload: jsonBody})
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "lmm.go : error invoke function [%s] with body %s for userId [%s] (equest new part %v) : %v",
			functionName, jsonBody, userId, requestNewPart, err)
		return commons.InternalLMMResp{}, false, commons.InternalServerError
	}

	if *resp.StatusCode != 200 {
		apimodel.Anlogger.Errorf(lc, "lmm.go : status code = %d, response body %s for request %s, for userId [%s] (function name %s, request new part %v)",
			*resp.StatusCode, string(resp.Payload), jsonBody, userId, functionName, requestNewPart)
		return commons.InternalLMMResp{}, false, commons.InternalServerError
	}

	var response commons.InternalLMMResp
	err = json.Unmarshal(resp.Payload, &response)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "lmm.go : error unmarshaling response %s into json for userId [%s] (function name %s, request new part %v) : %v",
			string(resp.Payload), userId, functionName, requestNewPart, err)
		return commons.InternalLMMResp{}, false, commons.InternalServerError
	}

	apimodel.Anlogger.Debugf(lc, "lmm.go : successfully got profiles for userId [%s] (function name %s, request new part %v), resp %v",
		userId, functionName, requestNewPart, response)
	return response, true, ""
}

func main() {
	basicLambda.Start(handler)
}
