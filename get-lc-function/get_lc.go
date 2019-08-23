package main

import (
	"../apimodel"
	basicLambda "github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/events"
	"context"
	"strings"
	"github.com/ringoid/commons"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"encoding/json"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"sync"
)

const (
	getLcEachFeedMaxLimit = 150
)

func init() {
	apimodel.InitLambdaVars("get-lc-feed")
}

func getLc(request *commons.GetLCRequest, functionName string, lc *lambdacontext.LambdaContext) (*commons.InternalGetLCResp, bool, string) {

	apimodel.Anlogger.Debugf(lc, "get_lc.go : get lc (function name %s) you for userId [%s]",
		functionName, *request.UserId)

	jsonBody, err := json.Marshal(request)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "get_lc.go : error marshaling req %s into json for userId [%s] (function name %s) : %v",
			request, *request.UserId, functionName, err)
		return nil, false, commons.InternalServerError
	}

	resp, err := apimodel.ClientLambda.Invoke(&lambda.InvokeInput{FunctionName: aws.String(functionName), Payload: jsonBody})
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "get_lc.go : error invoke function [%s] with body %s for userId [%s] : %v",
			functionName, jsonBody, *request.UserId, err)
		return nil, false, commons.InternalServerError
	}

	if *resp.StatusCode != 200 {
		apimodel.Anlogger.Errorf(lc, "get_lc.go : status code = %d, response body %s for request %s, for userId [%s] (function name %s",
			*resp.StatusCode, string(resp.Payload), jsonBody, *request.UserId, functionName)
		return nil, false, commons.InternalServerError
	}

	var response commons.InternalGetLCResp
	err = json.Unmarshal(resp.Payload, &response)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "get_lc.go : error unmarshaling response %s into json for userId [%s] (function name %s) : %v",
			string(resp.Payload), request.UserId, functionName, err)
		return nil, false, commons.InternalServerError
	}

	apimodel.Anlogger.Debugf(lc, "get_lc.go : successfully got profiles for userId [%s] (function name %s), resp %v",
		*request.UserId, functionName, response)
	return &response, true, ""
}

type TmpResult struct {
	GetLcFeedResp *apimodel.GetLcFeedResp
	Ok            bool
	ErrorStr      string
}

func handleJob(request *commons.GetLCRequest, isItLikes bool,
	innerResult *TmpResult,
	wg *sync.WaitGroup, lc *lambdacontext.LambdaContext) {

	defer wg.Done()

	functionName := apimodel.GetLcMessagesFunctionName
	if isItLikes {
		functionName = apimodel.GetLcLikesFunctionName
	}

	internalGetLcResponse, ok, errStr := getLc(request, functionName, lc)
	if !ok {
		innerResult.Ok = ok
		innerResult.ErrorStr = errStr
		return
	}

	if *request.LastActionTime > internalGetLcResponse.LastActionTime {
		innerResult.Ok = true
		innerResult.GetLcFeedResp.RepeatRequestAfter = apimodel.DefaultRepeatTimeSec
		apimodel.Anlogger.Debugf(lc, "get_lc.go : (%s) requested lastAction time [%v] > actual last actionTime [%v], diff [%v]",
			functionName, *request.LastActionTime, internalGetLcResponse.LastActionTime, internalGetLcResponse.LastActionTime - *request.LastActionTime)
		return
	}

	profiles := make([]commons.Profile, 0)
	for _, each := range internalGetLcResponse.Profiles {

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
			apimodel.Anlogger.Warnf(lc, "get_lc.go : lc return user [%s] with empty photo list for resolution [%s] for userId [%s]",
				each.UserId, *request.Resolution, *request.UserId)
			continue
		}

		lastOnlineText, lastOnlineFlag := apimodel.TransformLastOnlineTimeIntoStatusText(*request.UserId, each.LastOnlineTime, each.SourceLocale, lc)
		distanceText := apimodel.TransformDistanceInDistanceText(*request.UserId, each, lc)

		profile := commons.Profile{
			UserId:         each.UserId,
			Photos:         photos,
			Unseen:         each.Unseen,
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
			Children:       each.Children,
			Name:           each.Name,
			JobTitle:       each.JobTitle,
			Company:        each.Company,
			EducationText:  each.EducationText,
			About:          each.About,
			Instagram:      each.Instagram,
			TikTok:         each.TikTok,
			WhereLive:      each.WhereLive,
			WhereFrom:      each.WhereFrom,
			StatusText:     each.StatusText,
		}

		profile = apimodel.CheckProfileBeforeResponse(*request.UserId, profile)

		profiles = append(profiles, profile)
	}
	apimodel.Anlogger.Debugf(lc, "get_lc.go : prepare [%d] lc profiles for userId [%s]", len(profiles), *request.UserId)

	innerResult.Ok = true

	if isItLikes {
		innerResult.GetLcFeedResp.LikesYou = profiles
		innerResult.GetLcFeedResp.AllLikesYouProfilesNum = internalGetLcResponse.AllProfilesNum
		apimodel.Anlogger.Debugf(lc, "get_lc.go : set all likes you profile num to [%d] for userId [%s]", innerResult.GetLcFeedResp.AllLikesYouProfilesNum)
	} else {
		innerResult.GetLcFeedResp.Messages = profiles
		innerResult.GetLcFeedResp.AllMessagesProfilesNum = internalGetLcResponse.AllProfilesNum
		apimodel.Anlogger.Debugf(lc, "get_lc.go : set all messages/matches num to [%d] for userId [%s]", innerResult.GetLcFeedResp.AllMessagesProfilesNum)
	}
	return
}

func handler(ctx context.Context, request events.ALBTargetGroupRequest) (events.ALBTargetGroupResponse, error) {
	lc, _ := lambdacontext.FromContext(ctx)

	startTime := commons.UnixTimeInMillis()

	userAgent := request.Headers["user-agent"]
	if strings.HasPrefix(userAgent, "ELB-HealthChecker") {
		return commons.NewServiceResponse("{}"), nil
	}

	if request.HTTPMethod != "POST" {
		return commons.NewWrongHttpMethodServiceResponse(), nil
	}
	sourceIp := request.Headers["x-forwarded-for"]

	apimodel.Anlogger.Debugf(lc, "get_lc.go : start handle request %v", request)

	appVersion, isItAndroid, ok, errStr := commons.ParseAppVersionFromHeaders(request.Headers, apimodel.Anlogger, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "get_lc.go : return %s to client", errStr)
		return commons.NewServiceResponse(errStr), nil
	}

	reqParam, ok, errStr := parseParams(request.Body, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "get_lc.go : return %s to client", errStr)
		return commons.NewServiceResponse(errStr), nil
	}

	//todo:set hardcoded limit

	userId, ok, _, errStr := commons.CallVerifyAccessToken(appVersion, isItAndroid, *reqParam.AccessToken,
		apimodel.InternalAuthFunctionName, apimodel.ClientLambda, apimodel.Anlogger, lc)

	if !ok {
		apimodel.Anlogger.Errorf(lc, "get_lc.go : return %s to client", errStr)
		return commons.NewServiceResponse(errStr), nil
	}

	reqParam.UserId = &userId

	//prepare response
	feedResp := apimodel.GetLcFeedResp{}
	feedResp.LikesYou = make([]commons.Profile, 0)
	feedResp.Messages = make([]commons.Profile, 0)

	var commonWaitGroup sync.WaitGroup

	//likes you
	commonWaitGroup.Add(1)
	likeYouTmpResult := TmpResult{GetLcFeedResp: &apimodel.GetLcFeedResp{}}
	go handleJob(reqParam, true, &likeYouTmpResult, &commonWaitGroup, lc)

	//messages
	commonWaitGroup.Add(1)
	messagesTmpResult := TmpResult{GetLcFeedResp: &apimodel.GetLcFeedResp{}}
	go handleJob(reqParam, false, &messagesTmpResult, &commonWaitGroup, lc)

	commonWaitGroup.Wait()

	if !likeYouTmpResult.Ok {
		apimodel.Anlogger.Errorf(lc, "get_lc.go : userId [%s], return %s to client", userId, likeYouTmpResult.ErrorStr)
		return commons.NewServiceResponse(likeYouTmpResult.ErrorStr), nil
	}
	if !messagesTmpResult.Ok {
		apimodel.Anlogger.Errorf(lc, "get_lc.go : userId [%s], return %s to client", userId, messagesTmpResult.ErrorStr)
		return commons.NewServiceResponse(messagesTmpResult.ErrorStr), nil
	}

	if likeYouTmpResult.GetLcFeedResp.RepeatRequestAfter != 0 || messagesTmpResult.GetLcFeedResp.RepeatRequestAfter != 0 {
		apimodel.Anlogger.Debugf(lc, "get_lc.go : return repeat request after [%v] for userId [%s]", apimodel.DefaultRepeatTimeSec, userId)
		feedResp.RepeatRequestAfter = apimodel.DefaultRepeatTimeSec
	} else {
		feedResp.LikesYou = append(feedResp.LikesYou, likeYouTmpResult.GetLcFeedResp.LikesYou...)
		feedResp.AllLikesYouProfilesNum = likeYouTmpResult.GetLcFeedResp.AllLikesYouProfilesNum

		feedResp.Messages = append(feedResp.Messages, messagesTmpResult.GetLcFeedResp.Messages...)
		feedResp.AllMessagesProfilesNum = messagesTmpResult.GetLcFeedResp.AllMessagesProfilesNum
	}

	//mark sorting
	apimodel.MarkLCDefaultSort(userId, &feedResp, lc)

	body, err := json.Marshal(feedResp)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "get_lc.go : error while marshaling resp [%v] object for userId [%s] : %v", feedResp, userId, err)
		apimodel.Anlogger.Errorf(lc, "get_lc.go : userId [%s], return %s to client", userId, commons.InternalServerError)
		return commons.NewServiceResponse(commons.InternalServerError), nil
	}

	event := commons.NewProfileWasReturnToLCEvent(userId, sourceIp, *reqParam.Source, len(feedResp.LikesYou), len(feedResp.Messages), feedResp.RepeatRequestAfter)
	commons.SendAnalyticEvent(event, userId, apimodel.DeliveryStreamName, apimodel.AwsDeliveryStreamClient, apimodel.Anlogger, lc)

	finishTime := commons.UnixTimeInMillis()
	apimodel.Anlogger.Infof(lc, "get_lc.go : successfully return repeat request after [%v], [%d] likes you profiles and [%d] messages to userId [%s], duration [%v]", feedResp.RepeatRequestAfter, len(feedResp.LikesYou), len(feedResp.Messages), userId, finishTime-startTime)
	apimodel.Anlogger.Debugf(lc, "get_lc.go : return successful resp [%s] for userId [%s]", string(body), userId)
	return commons.NewServiceResponse(string(body)), nil
}

func parseParams(params string, lc *lambdacontext.LambdaContext) (*commons.GetLCRequest, bool, string) {
	apimodel.Anlogger.Debugf(lc, "get_lc.go : parse request body %s", params)

	var req commons.GetLCRequest
	err := json.Unmarshal([]byte(params), &req)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "get_lc.go : error marshaling required params from the string [%s] : %v", params, err)
		return nil, false, commons.InternalServerError
	}

	if req.AccessToken == nil || len(*req.AccessToken) == 0 {
		apimodel.Anlogger.Errorf(lc, "get_lc.go : accessToken is empty, request [%v]", req)
		return nil, false, commons.WrongRequestParamsClientError
	}

	if req.Resolution == nil || len(*req.Resolution) == 0 {
		apimodel.Anlogger.Errorf(lc, "get_lc.go : resolution is empty, request [%v]", req)
		return nil, false, commons.WrongRequestParamsClientError
	}

	if !commons.AllowedPhotoResolution[*req.Resolution] {
		apimodel.Anlogger.Warnf(lc, "get_lc.go : resolution [%s] is not supported, so use [%s] resolution", *req.Resolution, commons.BiggestDefaultPhotoResolution)
		req.Resolution = &commons.BiggestDefaultPhotoResolution
	}

	if req.LastActionTime == nil || *req.LastActionTime < 0 {
		apimodel.Anlogger.Errorf(lc, "get_lc.go : lastActionTime is empty or less than zero, request [%v]", req)
		return nil, false, commons.WrongRequestParamsClientError
	}

	if req.Source == nil {
		apimodel.Anlogger.Errorf(lc, "get_lc.go : source is empty, request [%v]", req)
		return nil, false, commons.WrongRequestParamsClientError
	}

	if _, ok := commons.FeedNames[*req.Source]; !ok && *req.Source != "profile" {
		apimodel.Anlogger.Errorf(lc, "get_lc.go : source contains unsupported value, request [%v]", req)
		return nil, false, commons.WrongRequestParamsClientError
	}

	if req.Filter != nil {

		if req.Filter.MinAge != nil {
			if *req.Filter.MinAge < 18 {
				age := 18
				req.Filter.MinAge = &age
			}
		} else {
			age := 18
			req.Filter.MinAge = &age
		}

		if req.Filter.MaxAge != nil {
			if *req.Filter.MaxAge < *req.Filter.MinAge {
				req.Filter.MaxAge = req.Filter.MinAge
			}
		}

		if req.Filter.MaxDistance != nil {
			if *req.Filter.MaxDistance < 1000 {
				dis := 1000
				req.Filter.MaxDistance = &dis
			}
		}
	}

	lim := getLcEachFeedMaxLimit
	req.Limit = &lim

	apimodel.Anlogger.Debugf(lc, "get_lc.go : successfully parse request [%v]", req)
	return &req, true, ""
}

func main() {
	basicLambda.Start(handler)
}
