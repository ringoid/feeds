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
)

const (
	newFacesDefaultLimit                   = 5
	newFacesMaxLimit                       = 100
	newFacesTimeToLiveLimitForViewRelInSec = 60 * 5
)

func init() {
	apimodel.InitLambdaVars("get-new-faces-feed")
}

func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	lc, _ := lambdacontext.FromContext(ctx)

	apimodel.Anlogger.Debugf(lc, "get_new_faces.go : start handle request %v", request)

	sourceIp := request.RequestContext.Identity.SourceIP

	if commons.IsItWarmUpRequest(request.Body, apimodel.Anlogger, lc) {
		return events.APIGatewayProxyResponse{}, nil
	}

	appVersion, isItAndroid, ok, errStr := commons.ParseAppVersionFromHeaders(request.Headers, apimodel.Anlogger, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : return %s to client", errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
	}

	accessToken, okA := request.QueryStringParameters["accessToken"]
	resolution, okR := request.QueryStringParameters["resolution"]
	lastActionTimeStr, okL := request.QueryStringParameters["lastActionTime"]

	if !okA || !okR || !okL {
		errStr = commons.WrongRequestParamsClientError
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : okA [%v], okR [%v] and okL [%v]", okA, okR, okL)
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : return %s to client", errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
	}

	limit := newFacesDefaultLimit
	limitStr := request.QueryStringParameters["limit"]
	var err error
	if len(limitStr) != 0 {
		limit, err = strconv.Atoi(limitStr)
		if err != nil {
			errStr = commons.WrongRequestParamsClientError
			apimodel.Anlogger.Errorf(lc, "get_new_faces.go : return %s to client", errStr)
			return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
		}
	}

	if !commons.AllowedPhotoResolution[resolution] {
		apimodel.Anlogger.Warnf(lc, "get_new_faces.go : resolution [%s] is not supported, so use [%s] resolution", resolution, commons.BiggestDefaultPhotoResolution)
		resolution = commons.BiggestDefaultPhotoResolution
	}

	lastActionTimeInt64, err := strconv.ParseInt(lastActionTimeStr, 10, 64)
	if err != nil {
		errStr := commons.WrongRequestParamsClientError
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : lastActionTime in wrong format [%s]", lastActionTimeStr)
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : return %s to client", errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
	}

	userId, ok, _, errStr := commons.CallVerifyAccessToken(appVersion, isItAndroid, accessToken, apimodel.InternalAuthFunctionName, apimodel.ClientLambda, apimodel.Anlogger, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : return %s to client", errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
	}

	internalNewFaces, repeatRequestAfter, ok, errStr := getNewFaces(userId, limit, lastActionTimeInt64, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : userId [%s], return %s to client", userId, errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
	}

	feedResp := apimodel.GetNewFacesFeedResp{}

	if repeatRequestAfter != 0 {
		feedResp.RepeatRequestAfter = repeatRequestAfter
	}

	targetIds := make([]string, 0)

	profiles := make([]commons.Profile, 0)
	for _, each := range internalNewFaces {
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
			UserId: each.UserId,
			Photos: photos,
		})

		targetIds = append(targetIds, each.UserId)
	}
	apimodel.Anlogger.Debugf(lc, "get_new_faces.go : prepare [%d] new faces profiles for userId [%s]", len(profiles), userId)
	resp := commons.ProfilesResp{}
	resp.Profiles = profiles

	//now enrich resp with photo uri
	resp, ok, errStr = enrichRespWithImageUrl(resp, userId, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : userId [%s], return %s to client", userId, errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
	}

	feedResp.Profiles = resp.Profiles

	//to simplify client logic lets remove possible nil objects
	if feedResp.Profiles == nil {
		feedResp.Profiles = make([]commons.Profile, 0)
	}

	apimodel.MarkNewFacesDefaultSort(userId, &feedResp, lc)

	body, err := json.Marshal(feedResp)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : error while marshaling resp [%v] object for userId [%s] : %v", feedResp, userId, err)
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : userId [%s], return %s to client", userId, commons.InternalServerError)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: commons.InternalServerError}, nil
	}

	event := commons.NewProfileWasReturnToNewFacesEvent(userId, sourceIp, targetIds, feedResp.RepeatRequestAfter)
	commons.SendAnalyticEvent(event, userId, apimodel.DeliveryStreamName, apimodel.AwsDeliveryStreamClient, apimodel.Anlogger, lc)
	//commons.SendCloudWatchMetric(apimodel.BaseCloudWatchNamespace, apimodel.NewFaceProfilesReturnMetricName, len(feedResp.Profiles), apimodel.AwsCWClient, apimodel.Anlogger, lc)

	apimodel.Anlogger.Infof(lc, "get_new_faces.go : successfully return [%d] new faces profiles to userId [%s]", len(feedResp.Profiles), userId)
	apimodel.Anlogger.Debugf(lc, "get_new_faces.go : return successful resp [%s] for userId [%s]", string(body), userId)
	return events.APIGatewayProxyResponse{StatusCode: 200, Body: string(body)}, nil
}

func enrichRespWithImageUrl(sourceResp commons.ProfilesResp, userId string, lc *lambdacontext.LambdaContext) (commons.ProfilesResp, bool, string) {
	apimodel.Anlogger.Debugf(lc, "get_new_faces.go : enrich response %v with image uri for userId [%s]", sourceResp, userId)
	if len(sourceResp.Profiles) == 0 {
		return commons.ProfilesResp{}, true, ""
	}

	jsonBody, err := json.Marshal(sourceResp)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : error marshaling source resp %s into json for userId [%s] : %v", sourceResp, userId, err)
		return commons.ProfilesResp{}, false, commons.InternalServerError
	}

	resp, err := apimodel.ClientLambda.Invoke(&lambda.InvokeInput{FunctionName: aws.String(apimodel.GetNewImagesInternalFunctionName), Payload: jsonBody})
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : error invoke function [%s] with body %s for userId [%s] : %v", apimodel.GetNewImagesInternalFunctionName, jsonBody, userId, err)
		return commons.ProfilesResp{}, false, commons.InternalServerError
	}

	if *resp.StatusCode != 200 {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : status code = %d, response body %s for request %s, for userId [%s] ", *resp.StatusCode, string(resp.Payload), jsonBody, userId)
		return commons.ProfilesResp{}, false, commons.InternalServerError
	}

	var response commons.FacesWithUrlResp
	err = json.Unmarshal(resp.Payload, &response)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : error unmarshaling response %s into json for userId [%s] : %v", string(resp.Payload), userId, err)
		return commons.ProfilesResp{}, false, commons.InternalServerError
	}

	apimodel.Anlogger.Debugf(lc, "get_new_faces.go : receive enriched with uri info from image service for userId [%s], map %v", userId, response)

	if len(response.UserIdPhotoIdKeyUrlMap) == 0 {
		apimodel.Anlogger.Warnf(lc, "get_new_faces.go : receive 0 image urls for userId [%s]", userId)
		return commons.ProfilesResp{}, true, ""
	}

	targetProfiles := make([]commons.Profile, 0)
	for _, eachProfile := range sourceResp.Profiles {
		sourceUserId := eachProfile.UserId
		//prepare Profile
		targetProfile := commons.Profile{}
		targetProfile.UserId = sourceUserId
		//it's new faces so always unseen
		targetProfile.Unseen = true
		targetPhotos := make([]commons.Photo, 0)
		//apimodel.Anlogger.Debugf(lc, "get_new_faces.go : construct photo slice for targetProfileId [%s], userId [%s]", targetProfile.UserId, userId)
		//now fill profile info
		for _, eachPhoto := range eachProfile.Photos {
			sourcePhotoId := eachPhoto.PhotoId
			apimodel.Anlogger.Debugf(lc, "get_new_faces.go : check photo with photoId [%s], userId [%s]", sourcePhotoId, userId)
			//construct key for map which we receive from images service
			targetMapKey := sourceUserId + "_" + sourcePhotoId
			if targetPhotoUri, ok := response.UserIdPhotoIdKeyUrlMap[targetMapKey]; ok {
				//apimodel.Anlogger.Debugf(lc, "get_new_faces.go : "+
				//	"found photoUri by key [%s] with photoId [%s] for targetProfileId [%s], userId [%s]",
				//	targetMapKey, sourcePhotoId, targetProfile.UserId, userId)

				//it means that we have photo uri in response from image service
				targetPhotos = append(targetPhotos, commons.Photo{
					PhotoId:  sourcePhotoId,
					PhotoUri: targetPhotoUri,
				})
			} else {
				apimodel.Anlogger.Debugf(lc, "get_new_faces.go : "+
					"didn't find photoUri by key [%s] with photoId [%s] for targetProfileId [%s], userId [%s]",
					targetMapKey, sourcePhotoId, targetProfile.UserId, userId)
			}
		}

		//now check should we put this profile in response
		targetProfile.Photos = targetPhotos
		if len(targetProfile.Photos) > 0 {
			apimodel.Anlogger.Debugf(lc, "get_new_faces.go : add profile with targetProfileId [%s] to the response with [%d] photos",
				targetProfile.UserId, len(targetProfile.Photos))
			targetProfiles = append(targetProfiles, targetProfile)
		} else {
			apimodel.Anlogger.Debugf(lc, "get_new_faces.go : skip profile with targetProfileId [%s], 0 photo uri", targetProfile.UserId)
		}
	}

	apimodel.Anlogger.Debugf(lc, "get_new_faces.go : successfully enrich response with photo uri for "+
		"userId [%s], profiles num [%d], resp %v", userId, len(targetProfiles), targetProfiles)
	sourceResp.Profiles = targetProfiles
	return sourceResp, true, ""
}

//response, repeat request after sec, ok and error string
func getNewFaces(userId string, limit int, lastActionTime int64, lc *lambdacontext.LambdaContext) ([]apimodel.InternalNewFace, int64, bool, string) {

	if limit < 0 {
		limit = newFacesDefaultLimit
	} else if limit > newFacesMaxLimit {
		limit = newFacesMaxLimit
	}
	apimodel.Anlogger.Debugf(lc, "get_new_faces.go : get new faces for userId [%s] with limit [%d]", userId, limit)

	req := apimodel.InternalGetNewFacesReq{
		UserId:         userId,
		Limit:          limit,
		LastActionTime: lastActionTime,
	}
	jsonBody, err := json.Marshal(req)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : error marshaling req %s into json for userId [%s] : %v", req, userId, err)
		return nil, 0, false, commons.InternalServerError
	}

	resp, err := apimodel.ClientLambda.Invoke(&lambda.InvokeInput{FunctionName: aws.String(apimodel.GetNewFacesFunctionName), Payload: jsonBody})
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : error invoke function [%s] with body %s for userId [%s] : %v", apimodel.GetNewFacesFunctionName, jsonBody, userId, err)
		return nil, 0, false, commons.InternalServerError
	}

	if *resp.StatusCode != 200 {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : status code = %d, response body %s for request %s, for userId [%s] ", *resp.StatusCode, string(resp.Payload), jsonBody, userId)
		return nil, 0, false, commons.InternalServerError
	}

	var response apimodel.InternalGetNewFacesResp
	err = json.Unmarshal(resp.Payload, &response)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : error unmarshaling response %s into json for userId [%s] : %v", string(resp.Payload), userId, err)
		return nil, 0, false, commons.InternalServerError
	}

	if lastActionTime > response.LastActionTime {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : requested lastActionTime [%d] > actual lastActionTime [%d] for userId [%s], diff is [%d]",
			lastActionTime, response.LastActionTime, userId, response.LastActionTime-lastActionTime)
		return nil, apimodel.DefaultRepeatTimeSec, true, ""
	}

	if len(response.NewFaces) == 0 {
		apimodel.Anlogger.Warnf(lc, "get_new_faces.go : got 0 profiles from relationships storage for userId [%s] with limit [%d]", userId, limit)
	}

	apimodel.Anlogger.Debugf(lc, "get_new_faces.go : successfully got new faces for userId [%s] with limit [%d], resp %v", userId, limit, response)
	return response.NewFaces, 0, true, ""
}

func main() {
	basicLambda.Start(handler)
}
