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
)

func init() {
	apimodel.InitLambdaVars("lmm-feed")
}

func handleJob(userId, resolution string, lastActionTimeInt int64, requestNewPart bool, functionName string, innerResult *InnerLmmResult,
	wg *sync.WaitGroup, lc *lambdacontext.LambdaContext) {
	defer wg.Done()

	llmResult, ok, errStr := llm(userId, functionName, requestNewPart, lastActionTimeInt, lc)
	if !ok {
		innerResult.ok = ok
		innerResult.errStr = errStr
		return
	}

	if lastActionTimeInt > llmResult.LastActionTime {
		apimodel.Anlogger.Warnf(lc, "lmm.go : requested lastActionTime [%d] > actual lastActionTime [%d] for userId [%s], diff is [%d]",
			lastActionTimeInt, llmResult.LastActionTime, userId, llmResult.LastActionTime-lastActionTimeInt)

		innerResult.ok = true
		innerResult.repeatRequestAfter = apimodel.DefaultRepeatTimeSec

		return
	}

	targetIds := make([]string, 0)

	profiles := make([]commons.Profile, 0)
	for _, each := range llmResult.Profiles {
		photos := make([]commons.Photo, 0)
		for _, eachPhoto := range each.PhotoIds {
			resolutionPhotoId, ok := commons.GetResolutionPhotoId(userId, eachPhoto, resolution, apimodel.Anlogger, lc)
			if ok {
				photos = append(photos, commons.Photo{
					PhotoId: resolutionPhotoId,
				})
			}
		}
		profiles = append(profiles, commons.Profile{
			UserId:   each.UserId,
			Photos:   photos,
			Unseen:   requestNewPart,
			Messages: make([]commons.Message, 0),
		})

		targetIds = append(targetIds, each.UserId)
	}
	apimodel.Anlogger.Debugf(lc, "lmm.go : prepare [%d] likes you profiles for userId [%s]", len(profiles), userId)

	resp := commons.ProfilesResp{}
	resp.Profiles = profiles

	//now enrich resp with photo uri
	resp, ok, errStr = enrichRespWithImageUrl(resp, userId, lc)
	if !ok {
		innerResult.ok = ok
		innerResult.errStr = errStr
		return
	}

	//only for messages
	if functionName == apimodel.MessagesFunctionName {
		enrichProfiles, ok, errStr := enrichWithMessages(resp.Profiles, userId, lc)
		if !ok {
			innerResult.ok = ok
			innerResult.errStr = errStr
			return
		}
		resp.Profiles = enrichProfiles
	}

	innerResult.ok = true
	innerResult.profiles = resp.Profiles

	return
}

type InnerLmmResult struct {
	ok                 bool
	errStr             string
	repeatRequestAfter int64
	profiles           []commons.Profile
}

func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	lc, _ := lambdacontext.FromContext(ctx)

	apimodel.Anlogger.Debugf(lc, "lmm.go : start handle request %v", request)

	sourceIp := request.RequestContext.Identity.SourceIP

	if commons.IsItWarmUpRequest(request.Body, apimodel.Anlogger, lc) {
		return events.APIGatewayProxyResponse{}, nil
	}

	appVersion, isItAndroid, ok, errStr := commons.ParseAppVersionFromHeaders(request.Headers, apimodel.Anlogger, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "lmm.go : return %s to client", errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
	}

	accessToken, okA := request.QueryStringParameters["accessToken"]
	resolution, okR := request.QueryStringParameters["resolution"]
	lastActionTimeStr, okL := request.QueryStringParameters["lastActionTime"]

	if !okA || !okR || !okL {
		errStr = commons.WrongRequestParamsClientError
		apimodel.Anlogger.Errorf(lc, "lmm.go : okA [%v], okR [%v] and okL [%v]", okA, okR, okL)
		apimodel.Anlogger.Errorf(lc, "lmm.go : return %s to client", errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
	}

	if !commons.AllowedPhotoResolution[resolution] {
		apimodel.Anlogger.Warnf(lc, "lmm.go : resolution [%s] is not supported, so use [%s] resolution", resolution, commons.BiggestDefaultPhotoResolution)
		resolution = commons.BiggestDefaultPhotoResolution
	}

	lastActionTimeInt64, err := strconv.ParseInt(lastActionTimeStr, 10, 64)
	if err != nil {
		errStr := commons.WrongRequestParamsClientError
		apimodel.Anlogger.Errorf(lc, "lmm.go : lastActionTime in wrong format [%s]", lastActionTimeStr)
		apimodel.Anlogger.Errorf(lc, "lmm.go : return %s to client", errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
	}

	userId, ok, _, errStr := commons.CallVerifyAccessToken(appVersion, isItAndroid, accessToken, apimodel.InternalAuthFunctionName, apimodel.ClientLambda, apimodel.Anlogger, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "lmm.go : return %s to client", errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
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
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: likesYouNewPart.errStr}, nil
	}

	if likesYouNewPart.repeatRequestAfter != 0 || likesYouOldPart.repeatRequestAfter != 0 ||
		matchesNewPart.repeatRequestAfter != 0 || matchesOldPart.repeatRequestAfter != 0 ||
		messagesPart.repeatRequestAfter != 0 {
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
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: commons.InternalServerError}, nil
	}

	event := commons.NewProfileWasReturnToLMMEvent(userId, sourceIp, len(feedResp.LikesYou), len(feedResp.Matches), len(feedResp.Messages), feedResp.RepeatRequestAfter)
	commons.SendAnalyticEvent(event, userId, apimodel.DeliveryStreamName, apimodel.AwsDeliveryStreamClient, apimodel.Anlogger, lc)

	//commons.SendCloudWatchMetric(apimodel.BaseCloudWatchNamespace, apimodel.LikesYouProfilesReturnMetricName, len(feedResp.LikesYou), apimodel.AwsCWClient, apimodel.Anlogger, lc)
	//commons.SendCloudWatchMetric(apimodel.BaseCloudWatchNamespace, apimodel.MatchProfilesReturnMetricName, len(feedResp.Matches), apimodel.AwsCWClient, apimodel.Anlogger, lc)
	//commons.SendCloudWatchMetric(apimodel.BaseCloudWatchNamespace, apimodel.MessageProfilesReturnMetricName, len(feedResp.Messages), apimodel.AwsCWClient, apimodel.Anlogger, lc)

	apimodel.Anlogger.Infof(lc, "lmm.go : successfully return [%d] likes you profiles, [%d] matches to userId [%s]", len(feedResp.LikesYou), len(feedResp.Matches), userId)
	apimodel.Anlogger.Debugf(lc, "lmm.go : return successful resp [%s] for userId [%s]", string(body), userId)
	return events.APIGatewayProxyResponse{StatusCode: 200, Body: string(body)}, nil
}

func enrichWithMessages(profiles []commons.Profile, userId string, lc *lambdacontext.LambdaContext) ([]commons.Profile, bool, string) {
	apimodel.Anlogger.Debugf(lc, "lmm.go : enrich message's response with actual message list for userId [%s]", userId)
	if len(profiles) == 0 {
		return profiles, true, ""
	}

	internalReq := commons.InternalGetMessagesReq{
		SourceUserId:  userId,
		TargetUserIds: make([]string, 0),
	}
	for _, each := range profiles {
		internalReq.TargetUserIds = append(internalReq.TargetUserIds, each.UserId)
	}

	jsonBody, err := json.Marshal(internalReq)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "lmm.go : error marshaling source request %s into json for userId [%s] : %v", internalReq, userId, err)
		return profiles, false, commons.InternalServerError
	}

	resp, err := apimodel.ClientLambda.Invoke(&lambda.InvokeInput{FunctionName: aws.String(apimodel.MessageContentFunctionName), Payload: jsonBody})
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "lmm.go : error invoke function [%s] with body %s for userId [%s] : %v", apimodel.MessageContentFunctionName, jsonBody, userId, err)
		return profiles, false, commons.InternalServerError
	}

	if *resp.StatusCode != 200 {
		apimodel.Anlogger.Errorf(lc, "lmm.go : status code = %d, response body %s for request %s, for userId [%s] ", *resp.StatusCode, string(resp.Payload), jsonBody, userId)
		return profiles, false, commons.InternalServerError
	}

	var response commons.InternalGetMessagesResp
	err = json.Unmarshal(resp.Payload, &response)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "lmm.go : error unmarshaling response %s into json for userId [%s] : %v", string(resp.Payload), userId, err)
		return profiles, false, commons.InternalServerError
	}

	for index, each := range profiles {
		if msgs, ok := response.ConversationsMap[each.UserId]; ok {
			profiles[index].Messages = msgs
		}
	}

	apimodel.Anlogger.Debugf(lc, "lmm.go : successfully enrich message's response with actual message list for userId [%s]", userId)
	return profiles, true, ""
}

func enrichRespWithImageUrl(sourceResp commons.ProfilesResp, userId string, lc *lambdacontext.LambdaContext) (commons.ProfilesResp, bool, string) {
	apimodel.Anlogger.Debugf(lc, "lmm.go : enrich response with image uri for userId [%s]", userId)
	if len(sourceResp.Profiles) == 0 {
		return commons.ProfilesResp{}, true, ""
	}

	jsonBody, err := json.Marshal(sourceResp)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "lmm.go : error marshaling source resp %s into json for userId [%s] : %v", sourceResp, userId, err)
		return commons.ProfilesResp{}, false, commons.InternalServerError
	}

	resp, err := apimodel.ClientLambda.Invoke(&lambda.InvokeInput{FunctionName: aws.String(apimodel.GetNewImagesInternalFunctionName), Payload: jsonBody})
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "lmm.go : error invoke function [%s] with body %s for userId [%s] : %v", apimodel.GetNewImagesInternalFunctionName, jsonBody, userId, err)
		return commons.ProfilesResp{}, false, commons.InternalServerError
	}

	if *resp.StatusCode != 200 {
		apimodel.Anlogger.Errorf(lc, "lmm.go : status code = %d, response body %s for request %s, for userId [%s] ", *resp.StatusCode, string(resp.Payload), jsonBody, userId)
		return commons.ProfilesResp{}, false, commons.InternalServerError
	}

	var response commons.FacesWithUrlResp
	err = json.Unmarshal(resp.Payload, &response)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "lmm.go : error unmarshaling response %s into json for userId [%s] : %v", string(resp.Payload), userId, err)
		return commons.ProfilesResp{}, false, commons.InternalServerError
	}

	apimodel.Anlogger.Debugf(lc, "lmm.go : receive enriched with uri info from image service for userId [%s], map %v", userId, response)

	if len(response.UserIdPhotoIdKeyUrlMap) == 0 {
		apimodel.Anlogger.Warnf(lc, "lmm.go : receive 0 image urls for userId [%s]", userId)
		return commons.ProfilesResp{}, true, ""
	}

	targetProfiles := make([]commons.Profile, 0)
	for _, eachProfile := range sourceResp.Profiles {
		sourceUserId := eachProfile.UserId
		//prepare Profile
		targetProfile := commons.Profile{}
		targetProfile.UserId = sourceUserId
		targetProfile.Unseen = eachProfile.Unseen
		targetProfile.Messages = eachProfile.Messages

		targetPhotos := make([]commons.Photo, 0)
		apimodel.Anlogger.Debugf(lc, "lmm.go : construct photo slice for targetProfileId [%s], userId [%s]", targetProfile.UserId, userId)
		//now fill profile info
		for _, eachPhoto := range eachProfile.Photos {
			sourcePhotoId := eachPhoto.PhotoId
			apimodel.Anlogger.Debugf(lc, "lmm.go : check photo with photoId [%s], userId [%s]", sourcePhotoId, userId)
			//construct key for map which we receive from images service
			targetMapKey := sourceUserId + "_" + sourcePhotoId
			if targetPhotoUri, ok := response.UserIdPhotoIdKeyUrlMap[targetMapKey]; ok {
				apimodel.Anlogger.Debugf(lc, "lmm.go : "+
					"found photoUri by key [%s] with photoId [%s] for targetProfileId [%s], userId [%s]",
					targetMapKey, sourcePhotoId, targetProfile.UserId, userId)
				//it means that we have photo uri in response from image service
				targetPhotos = append(targetPhotos, commons.Photo{
					PhotoId:  sourcePhotoId,
					PhotoUri: targetPhotoUri,
				})
			} else {
				apimodel.Anlogger.Debugf(lc, "lmm.go : "+
					"didn't find photoUri by key [%s] with photoId [%s] for targetProfileId [%s], userId [%s]",
					targetMapKey, sourcePhotoId, targetProfile.UserId, userId)
			}
		}

		//now check should we put this profile in response
		targetProfile.Photos = targetPhotos
		if len(targetProfile.Photos) > 0 {
			apimodel.Anlogger.Debugf(lc, "lmm.go : add profile with targetProfileId [%s] to the response with [%d] photos",
				targetProfile.UserId, len(targetProfile.Photos))
			targetProfiles = append(targetProfiles, targetProfile)
		} else {
			apimodel.Anlogger.Debugf(lc, "lmm.go : skip profile with targetProfileId [%s], 0 photo uri", targetProfile.UserId)
		}
	}

	apimodel.Anlogger.Debugf(lc, "lmm.go : successfully enrich response with photo uri for "+
		"userId [%s], profiles num [%d], resp %v", userId, len(targetProfiles), targetProfiles)
	sourceResp.Profiles = targetProfiles
	return sourceResp, true, ""
}

func llm(userId, functionName string, requestNewPart bool, lastActionTime int64, lc *lambdacontext.LambdaContext) (commons.InternalLMMResp, bool, string) {

	apimodel.Anlogger.Debugf(lc, "lmm.go : get llm (function name %s, request new part %v) you for userId [%s]", functionName, requestNewPart, userId)

	req := commons.InternalLMMReq{
		UserId:                  userId,
		RequestNewPart:          requestNewPart,
		RequestedLastActionTime: lastActionTime,
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

	if len(response.Profiles) == 0 {
		apimodel.Anlogger.Warnf(lc, "lmm.go : got 0 profiles from relationships storage for userId [%s] (function name %s, request new part %v)",
			userId, functionName, requestNewPart)
	}

	apimodel.Anlogger.Debugf(lc, "lmm.go : successfully got profiles for userId [%s] (function name %s, request new part %v), resp %v",
		userId, functionName, requestNewPart, response)
	return response, true, ""
}

func main() {
	basicLambda.Start(handler)
}
