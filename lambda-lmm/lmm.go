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

const (
	defaultRepeatTimeSec = 2
)

func init() {
	apimodel.InitLambdaVars("lmm-feed")
}

func handleJob(userId, resolution string, lastActionTimeInt int, requestNewPart bool, functionName string, innerResult *InnerResult,
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
		innerResult.repeatRequestAfterSec = defaultRepeatTimeSec

		return
	}

	targetIds := make([]string, 0)

	profiles := make([]apimodel.Profile, 0)
	for _, each := range llmResult.Profiles {
		photos := make([]apimodel.Photo, 0)
		for _, eachPhoto := range each.PhotoIds {
			resolutionPhotoId, ok := commons.GetResolutionPhotoId(userId, eachPhoto, resolution, apimodel.Anlogger, lc)
			if ok {
				photos = append(photos, apimodel.Photo{
					PhotoId: resolutionPhotoId,
				})
			}
		}
		profiles = append(profiles, apimodel.Profile{
			UserId: each.UserId,
			Photos: photos,
		})

		targetIds = append(targetIds, each.UserId)
	}
	apimodel.Anlogger.Debugf(lc, "lmm.go : prepare [%d] likes you profiles for userId [%s]", len(profiles), userId)

	resp := apimodel.ProfilesResp{}
	resp.Profiles = profiles

	//now enrich resp with photo uri
	resp, ok, errStr = enrichRespWithImageUrl(resp, userId, lc)
	if !ok {
		innerResult.ok = ok
		innerResult.errStr = errStr
		return
	}

	innerResult.ok = true
	innerResult.profiles = resp.Profiles

	return
}

type InnerResult struct {
	ok                    bool
	errStr                string
	repeatRequestAfterSec int
	profiles              []apimodel.Profile
}

func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	lc, _ := lambdacontext.FromContext(ctx)

	apimodel.Anlogger.Debugf(lc, "lmm.go : start handle request %v", request)

	if commons.IsItWarmUpRequest(request.Body, apimodel.Anlogger, lc) {
		return events.APIGatewayProxyResponse{}, nil
	}

	appVersion, isItAndroid, ok, errStr := commons.ParseAppVersionFromHeaders(request.Headers, apimodel.Anlogger, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "lmm.go : return %s to client", errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
	}

	accessToken := request.QueryStringParameters["accessToken"]
	resolution := request.QueryStringParameters["resolution"]
	lastActionTimeStr := request.QueryStringParameters["lastActionTime"]

	if !commons.AllowedPhotoResolution[resolution] {
		errStr := commons.WrongRequestParamsClientError
		apimodel.Anlogger.Errorf(lc, "lmm.go : resolution [%s] is not supported", resolution)
		apimodel.Anlogger.Errorf(lc, "lmm.go : return %s to client", errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
	}

	lastActionTimeInt, err := strconv.Atoi(lastActionTimeStr)
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
	feedResp.LikesYouNewProfiles = make([]apimodel.Profile, 0)
	feedResp.LikesYouOldProfiles = make([]apimodel.Profile, 0)
	feedResp.MatchesNewProfiles = make([]apimodel.Profile, 0)
	feedResp.MatchesOldProfiles = make([]apimodel.Profile, 0)

	var commonWaitGroup sync.WaitGroup

	//likes you (new part)
	commonWaitGroup.Add(1)
	likesYouNewPart := InnerResult{}
	go handleJob(userId, resolution, lastActionTimeInt, true, apimodel.LikesYouFunctionName, &likesYouNewPart,
		&commonWaitGroup, lc)

	//likes you (old part)
	commonWaitGroup.Add(1)
	likesYouOldPart := InnerResult{}
	go handleJob(userId, resolution, lastActionTimeInt, false, apimodel.LikesYouFunctionName, &likesYouOldPart,
		&commonWaitGroup, lc)

	//matches (new part)
	commonWaitGroup.Add(1)
	matchesNewPart := InnerResult{}
	go handleJob(userId, resolution, lastActionTimeInt, true, apimodel.MatchesFunctionName, &matchesNewPart,
		&commonWaitGroup, lc)

	//matches (old part)
	commonWaitGroup.Add(1)
	matchesOldPart := InnerResult{}
	go handleJob(userId, resolution, lastActionTimeInt, false, apimodel.MatchesFunctionName, &matchesOldPart,
		&commonWaitGroup, lc)

	commonWaitGroup.Wait()

	if !likesYouNewPart.ok || !likesYouOldPart.ok || !matchesNewPart.ok || !matchesOldPart.ok {
		//todo:find real error
		apimodel.Anlogger.Errorf(lc, "lmm.go : userId [%s], return %s to client", userId, likesYouNewPart.errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: likesYouNewPart.errStr}, nil
	}

	if likesYouNewPart.repeatRequestAfterSec != 0 || likesYouOldPart.repeatRequestAfterSec != 0 ||
		matchesNewPart.repeatRequestAfterSec != 0 || matchesOldPart.repeatRequestAfterSec != 0 {
		feedResp.RepeatRequestAfterSec = defaultRepeatTimeSec
	} else {
		feedResp.LikesYouNewProfiles = likesYouNewPart.profiles
		feedResp.LikesYouOldProfiles = likesYouOldPart.profiles
		feedResp.MatchesNewProfiles = matchesNewPart.profiles
		feedResp.MatchesOldProfiles = matchesOldPart.profiles
	}

	body, err := json.Marshal(feedResp)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "lmm.go : error while marshaling resp [%v] object for userId [%s] : %v", feedResp, userId, err)
		apimodel.Anlogger.Errorf(lc, "lmm.go : userId [%s], return %s to client", userId, commons.InternalServerError)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: commons.InternalServerError}, nil
	}
	//todo:think about analytics
	//commons.SendAnalyticEvent(event, userId, apimodel.DeliveryStramName, apimodel.AwsDeliveryStreamClient, apimodel.Anlogger, lc)

	apimodel.Anlogger.Infof(lc, "lmm.go : successfully return [%d] likes you profiles to userId [%s]", len(feedResp.LikesYouNewProfiles), userId)
	apimodel.Anlogger.Debugf(lc, "lmm.go : return successful resp [%s] for userId [%s]", string(body), userId)
	return events.APIGatewayProxyResponse{StatusCode: 200, Body: string(body)}, nil
}

func enrichRespWithImageUrl(sourceResp apimodel.ProfilesResp, userId string, lc *lambdacontext.LambdaContext) (apimodel.ProfilesResp, bool, string) {
	apimodel.Anlogger.Debugf(lc, "lmm.go : enrich response %v with image uri for userId [%s]", sourceResp, userId)
	if len(sourceResp.Profiles) == 0 {
		return apimodel.ProfilesResp{}, true, ""
	}

	jsonBody, err := json.Marshal(sourceResp)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "lmm.go : error marshaling source resp %s into json for userId [%s] : %v", sourceResp, userId, err)
		return apimodel.ProfilesResp{}, false, commons.InternalServerError
	}

	resp, err := apimodel.ClientLambda.Invoke(&lambda.InvokeInput{FunctionName: aws.String(apimodel.GetNewImagesInternalFunctionName), Payload: jsonBody})
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "lmm.go : error invoke function [%s] with body %s for userId [%s] : %v", apimodel.GetNewImagesInternalFunctionName, jsonBody, userId, err)
		return apimodel.ProfilesResp{}, false, commons.InternalServerError
	}

	if *resp.StatusCode != 200 {
		apimodel.Anlogger.Errorf(lc, "lmm.go : status code = %d, response body %s for request %s, for userId [%s] ", *resp.StatusCode, string(resp.Payload), jsonBody, userId)
		return apimodel.ProfilesResp{}, false, commons.InternalServerError
	}

	var response apimodel.FacesWithUrlResp
	err = json.Unmarshal(resp.Payload, &response)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "lmm.go : error unmarshaling response %s into json for userId [%s] : %v", string(resp.Payload), userId, err)
		return apimodel.ProfilesResp{}, false, commons.InternalServerError
	}

	apimodel.Anlogger.Debugf(lc, "lmm.go : receive enriched with uri info from image service for userId [%s], map %v", userId, response)

	if len(response.UserIdPhotoIdKeyUrlMap) == 0 {
		apimodel.Anlogger.Warnf(lc, "lmm.go : receive 0 image urls for userId [%s]", userId)
		return apimodel.ProfilesResp{}, true, ""
	}

	targetProfiles := make([]apimodel.Profile, 0)
	for _, eachProfile := range sourceResp.Profiles {
		sourceUserId := eachProfile.UserId
		//prepare Profile
		targetProfile := apimodel.Profile{}
		targetProfile.UserId = sourceUserId
		targetPhotos := make([]apimodel.Photo, 0)
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
				targetPhotos = append(targetPhotos, apimodel.Photo{
					PhotoId:  sourcePhotoId,
					PhotoUri: targetPhotoUri,
				})
			} else {
				apimodel.Anlogger.Debugf(lc, "lmm.go : "+
					"didn't find photoUri by key [%s] with photoId [%s] for targetProfileId [%s], userId [%s]",
					targetMapKey, sourcePhotoId, targetProfile.UserId, userId)
			}
			//todo:delete, need for debug
			apimodel.Anlogger.Debugf(lc, "lmm.go : after checking photo with photoId [%s], len(targetPhotos)==%d", sourcePhotoId, len(targetPhotos))
		}
		//todo:delete, need for debug
		apimodel.Anlogger.Debugf(lc, "lmm.go : after checking all photos for targetProfileId [%s], len(targetPhotos)==%d, len(targetProfile.Photos)==%d",
			targetProfile.UserId, len(targetPhotos), len(targetProfile.Photos))

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

func llm(userId, functionName string, requestNewPart bool, lastActionTime int, lc *lambdacontext.LambdaContext) (apimodel.InternalLMMResp, bool, string) {

	apimodel.Anlogger.Debugf(lc, "lmm.go : get likes you for userId [%s]", userId)

	req := apimodel.InternalLMMReq{
		UserId:                  userId,
		RequestNewPart:          requestNewPart,
		RequestedLastActionTime: lastActionTime,
	}
	jsonBody, err := json.Marshal(req)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "lmm.go : error marshaling req %s into json for userId [%s] : %v", req, userId, err)
		return apimodel.InternalLMMResp{}, false, commons.InternalServerError
	}

	resp, err := apimodel.ClientLambda.Invoke(&lambda.InvokeInput{FunctionName: aws.String(functionName), Payload: jsonBody})
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "lmm.go : error invoke function [%s] with body %s for userId [%s] : %v", functionName, jsonBody, userId, err)
		return apimodel.InternalLMMResp{}, false, commons.InternalServerError
	}

	if *resp.StatusCode != 200 {
		apimodel.Anlogger.Errorf(lc, "lmm.go : status code = %d, response body %s for request %s, for userId [%s] ", *resp.StatusCode, string(resp.Payload), jsonBody, userId)
		return apimodel.InternalLMMResp{}, false, commons.InternalServerError
	}

	var response apimodel.InternalLMMResp
	err = json.Unmarshal(resp.Payload, &response)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "lmm.go : error unmarshaling response %s into json for userId [%s] : %v", string(resp.Payload), userId, err)
		return apimodel.InternalLMMResp{}, false, commons.InternalServerError
	}

	if len(response.Profiles) == 0 {
		apimodel.Anlogger.Warnf(lc, "lmm.go : got 0 profiles from relationships storage for userId [%s]", userId)
	}

	apimodel.Anlogger.Debugf(lc, "lmm.go : successfully got new faces for userId [%s], resp %v", userId, response)
	return response, true, ""
}

func main() {
	basicLambda.Start(handler)
}
