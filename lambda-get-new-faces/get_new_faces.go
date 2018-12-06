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
	"time"
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

	if commons.IsItWarmUpRequest(request.Body, apimodel.Anlogger, lc) {
		return events.APIGatewayProxyResponse{}, nil
	}

	appVersion, isItAndroid, ok, errStr := commons.ParseAppVersionFromHeaders(request.Headers, apimodel.Anlogger, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : return %s to client", errStr)
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
			errStr = commons.WrongRequestParamsClientError
			apimodel.Anlogger.Errorf(lc, "get_new_faces.go : return %s to client", errStr)
			return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
		}
	}

	if !commons.AllowedPhotoResolution[resolution] {
		errStr := commons.WrongRequestParamsClientError
		apimodel.Anlogger.Errorf(lc, "get_new_faces : resolution [%s] is not supported", resolution)
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : return %s to client", errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
	}

	userId, ok, _, errStr := commons.CallVerifyAccessToken(appVersion, isItAndroid, accessToken, apimodel.InternalAuthFunctionName, apimodel.ClientLambda, apimodel.Anlogger, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : return %s to client", errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
	}

	internalNewFaces, ok, errStr := getNewFaces(userId, limit, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : userId [%s], return %s to client", userId, errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
	}

	targetIds := make([]string, 0)

	profiles := make([]apimodel.Profile, 0)
	for _, each := range internalNewFaces {
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
	apimodel.Anlogger.Debugf(lc, "get_new_faces.go : prepare [%d] new faces profiles for userId [%s]", len(profiles), userId)
	resp := apimodel.ProfilesResp{}
	resp.Profiles = profiles

	timeToDeleteViewRel := time.Now().Unix() + newFacesTimeToLiveLimitForViewRelInSec
	event := commons.NewProfileWasReturnToNewFacesEvent(userId, timeToDeleteViewRel, targetIds)
	ok, errStr = commons.SendCommonEvent(event, userId, apimodel.CommonStreamName, userId, apimodel.AwsKinesisClient, apimodel.Anlogger, lc)
	if !ok {
		errStr := commons.InternalServerError
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : userId [%s], return %s to client", userId, errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
	}

	commons.SendAnalyticEvent(event, userId, apimodel.DeliveryStramName, apimodel.AwsDeliveryStreamClient, apimodel.Anlogger, lc)

	//now enrich resp with photo uri
	resp, ok, errStr = enrichRespWithImageUrl(resp, userId, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : userId [%s], return %s to client", userId, errStr)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: errStr}, nil
	}

	feedResp := apimodel.GetNewFacesFeedResp{
		Profiles: resp.Profiles,
	}

	//to simplify client logic lets remove possible nil objects
	if feedResp.Profiles == nil {
		feedResp.Profiles = make([]apimodel.Profile, 0)
	}

	//mark sorting
	apimodel.MarkNewFacesDefaultSort(userId, &feedResp, lc)

	body, err := json.Marshal(feedResp)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : error while marshaling resp [%v] object for userId [%s] : %v", feedResp, userId, err)
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : userId [%s], return %s to client", userId, commons.InternalServerError)
		return events.APIGatewayProxyResponse{StatusCode: 200, Body: commons.InternalServerError}, nil
	}
	apimodel.Anlogger.Infof(lc, "get_new_faces.go : successfully return [%d] new faces profiles to userId [%s]", len(feedResp.Profiles), userId)
	apimodel.Anlogger.Debugf(lc, "get_new_faces.go : return successful resp [%s] for userId [%s]", string(body), userId)
	return events.APIGatewayProxyResponse{StatusCode: 200, Body: string(body)}, nil
}

func markDefaultSort(resp *apimodel.GetNewFacesFeedResp) *apimodel.GetNewFacesFeedResp {
	for index, eachP := range resp.Profiles {
		eachP.DefaultSortingOrderPosition = index
	}
	return resp
}

func enrichRespWithImageUrl(sourceResp apimodel.ProfilesResp, userId string, lc *lambdacontext.LambdaContext) (apimodel.ProfilesResp, bool, string) {
	apimodel.Anlogger.Debugf(lc, "get_new_faces.go : enrich response %v with image uri for userId [%s]", sourceResp, userId)
	if len(sourceResp.Profiles) == 0 {
		return apimodel.ProfilesResp{}, true, ""
	}

	jsonBody, err := json.Marshal(sourceResp)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : error marshaling source resp %s into json for userId [%s] : %v", sourceResp, userId, err)
		return apimodel.ProfilesResp{}, false, commons.InternalServerError
	}

	resp, err := apimodel.ClientLambda.Invoke(&lambda.InvokeInput{FunctionName: aws.String(apimodel.GetNewImagesInternalFunctionName), Payload: jsonBody})
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : error invoke function [%s] with body %s for userId [%s] : %v", apimodel.GetNewImagesInternalFunctionName, jsonBody, userId, err)
		return apimodel.ProfilesResp{}, false, commons.InternalServerError
	}

	if *resp.StatusCode != 200 {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : status code = %d, response body %s for request %s, for userId [%s] ", *resp.StatusCode, string(resp.Payload), jsonBody, userId)
		return apimodel.ProfilesResp{}, false, commons.InternalServerError
	}

	var response apimodel.FacesWithUrlResp
	err = json.Unmarshal(resp.Payload, &response)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : error unmarshaling response %s into json for userId [%s] : %v", string(resp.Payload), userId, err)
		return apimodel.ProfilesResp{}, false, commons.InternalServerError
	}

	apimodel.Anlogger.Debugf(lc, "get_new_faces.go : receive enriched with uri info from image service for userId [%s], map %v", userId, response)

	if len(response.UserIdPhotoIdKeyUrlMap) == 0 {
		apimodel.Anlogger.Warnf(lc, "get_new_faces.go : receive 0 image urls for userId [%s]", userId)
		return apimodel.ProfilesResp{}, true, ""
	}

	targetProfiles := make([]apimodel.Profile, 0)
	for _, eachProfile := range sourceResp.Profiles {
		sourceUserId := eachProfile.UserId
		//prepare Profile
		targetProfile := apimodel.Profile{}
		targetProfile.UserId = sourceUserId
		targetPhotos := make([]apimodel.Photo, 0)
		apimodel.Anlogger.Debugf(lc, "get_new_faces.go : construct photo slice for targetProfileId [%s], userId [%s]", targetProfile.UserId, userId)
		//now fill profile info
		for _, eachPhoto := range eachProfile.Photos {
			sourcePhotoId := eachPhoto.PhotoId
			apimodel.Anlogger.Debugf(lc, "get_new_faces.go : check photo with photoId [%s], userId [%s]", sourcePhotoId, userId)
			//construct key for map which we receive from images service
			targetMapKey := sourceUserId + "_" + sourcePhotoId
			if targetPhotoUri, ok := response.UserIdPhotoIdKeyUrlMap[targetMapKey]; ok {
				apimodel.Anlogger.Debugf(lc, "get_new_faces.go : "+
					"found photoUri by key [%s] with photoId [%s] for targetProfileId [%s], userId [%s]",
					targetMapKey, sourcePhotoId, targetProfile.UserId, userId)
				//it means that we have photo uri in response from image service
				targetPhotos = append(targetPhotos, apimodel.Photo{
					PhotoId:  sourcePhotoId,
					PhotoUri: targetPhotoUri,
				})
			} else {
				apimodel.Anlogger.Debugf(lc, "get_new_faces.go : "+
					"didn't find photoUri by key [%s] with photoId [%s] for targetProfileId [%s], userId [%s]",
					targetMapKey, sourcePhotoId, targetProfile.UserId, userId)
			}
			//todo:delete, need for debug
			apimodel.Anlogger.Debugf(lc, "get_new_faces.go : after checking photo with photoId [%s], len(targetPhotos)==%d", sourcePhotoId, len(targetPhotos))
		}
		//todo:delete, need for debug
		apimodel.Anlogger.Debugf(lc, "get_new_faces.go : after checking all photos for targetProfileId [%s], len(targetPhotos)==%d, len(targetProfile.Photos)==%d",
			targetProfile.UserId, len(targetPhotos), len(targetProfile.Photos))

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

func getNewFaces(userId string, limit int, lc *lambdacontext.LambdaContext) ([]apimodel.InternalNewFace, bool, string) {

	if limit < 0 {
		limit = newFacesDefaultLimit
	} else if limit > newFacesMaxLimit {
		limit = newFacesMaxLimit
	}
	apimodel.Anlogger.Debugf(lc, "get_new_faces.go : get new faces for userId [%s] with limit [%d]", userId, limit)

	req := apimodel.InternalGetNewFacesReq{
		UserId:
		userId,
		Limit: limit,
	}
	jsonBody, err := json.Marshal(req)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : error marshaling req %s into json for userId [%s] : %v", req, userId, err)
		return nil, false, commons.InternalServerError
	}

	resp, err := apimodel.ClientLambda.Invoke(&lambda.InvokeInput{FunctionName: aws.String(apimodel.GetNewFacesFunctionName), Payload: jsonBody})
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : error invoke function [%s] with body %s for userId [%s] : %v", apimodel.GetNewFacesFunctionName, jsonBody, userId, err)
		return nil, false, commons.InternalServerError
	}

	if *resp.StatusCode != 200 {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : status code = %d, response body %s for request %s, for userId [%s] ", *resp.StatusCode, string(resp.Payload), jsonBody, userId)
		return nil, false, commons.InternalServerError
	}

	var response apimodel.InternalGetNewFacesResp
	err = json.Unmarshal(resp.Payload, &response)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : error unmarshaling response %s into json for userId [%s] : %v", string(resp.Payload), userId, err)
		return nil, false, commons.InternalServerError
	}

	if len(response.NewFaces) == 0 {
		apimodel.Anlogger.Warnf(lc, "get_new_faces.go : got 0 profiles from relationships storage for userId [%s] with limit [%d]", userId, limit)
	}

	apimodel.Anlogger.Debugf(lc, "get_new_faces.go : successfully got new faces for userId [%s] with limit [%d], resp %v", userId, limit, response)
	return response.NewFaces, true, ""
}

func main() {
	basicLambda.Start(handler)
}
