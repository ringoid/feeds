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

const (
	newFacesDefaultLimit                   = 5
	newFacesMaxLimit                       = 100
	newFacesTimeToLiveLimitForViewRelInSec = 60 * 5
)

func init() {
	apimodel.InitLambdaVars("get-new-faces-feed")
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

	apimodel.Anlogger.Debugf(lc, "get_new_faces.go : start handle request %v", request)

	appVersion, isItAndroid, ok, errStr := commons.ParseAppVersionFromHeaders(request.Headers, apimodel.Anlogger, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : return %s to client", errStr)
		return commons.NewServiceResponse(errStr), nil
	}

	accessToken, okA := request.QueryStringParameters["accessToken"]
	resolution, okR := request.QueryStringParameters["resolution"]
	lastActionTimeStr, okL := request.QueryStringParameters["lastActionTime"]

	if !okA || !okR || !okL {
		errStr = commons.WrongRequestParamsClientError
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : okA [%v], okR [%v] and okL [%v]", okA, okR, okL)
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : return %s to client", errStr)
		return commons.NewServiceResponse(errStr), nil
	}

	limit := newFacesDefaultLimit
	limitStr := request.QueryStringParameters["limit"]
	var err error
	if len(limitStr) != 0 {
		limit, err = strconv.Atoi(limitStr)
		if err != nil {
			errStr = commons.WrongRequestParamsClientError
			apimodel.Anlogger.Errorf(lc, "get_new_faces.go : return %s to client", errStr)
			return commons.NewServiceResponse(errStr), nil
		}
	}

	if !commons.AllowedPhotoResolution[resolution] {
		apimodel.Anlogger.Warnf(lc, "get_new_faces.go : resolution [%s] is not supported, so use [%s] resolution", resolution, commons.BiggestDefaultPhotoResolution)
		resolution = commons.BiggestDefaultPhotoResolution
	}

	lastActionTimeInt64, err := strconv.ParseInt(lastActionTimeStr, 10, 64)
	if err != nil || lastActionTimeInt64 < 0 {
		errStr := commons.WrongRequestParamsClientError
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : lastActionTime in wrong format [%s]", lastActionTimeStr)
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : return %s to client", errStr)
		return commons.NewServiceResponse(errStr), nil
	}

	userId, ok, _, errStr := commons.CallVerifyAccessToken(appVersion, isItAndroid, accessToken, apimodel.InternalAuthFunctionName, apimodel.ClientLambda, apimodel.Anlogger, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : return %s to client", errStr)
		return commons.NewServiceResponse(errStr), nil
	}

	internalNewFaces, repeatRequestAfter, ok, errStr := getNewFaces(userId, limit, lastActionTimeInt64, resolution, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : userId [%s], return %s to client", userId, errStr)
		return commons.NewServiceResponse(errStr), nil
	}

	feedResp := apimodel.GetNewFacesFeedResp{}

	if repeatRequestAfter != 0 {
		feedResp.RepeatRequestAfter = repeatRequestAfter
	}

	targetIds := make([]string, 0)

	profiles := make([]commons.Profile, 0)
	for _, each := range internalNewFaces {

		photos := make([]commons.Photo, 0)

		for _, eachPhoto := range each.Photos {
			photos = append(photos, commons.Photo{
				PhotoId:           eachPhoto.ResizedPhotoId,
				PhotoUri:          eachPhoto.Link,
				ThumbnailPhotoUri: eachPhoto.ThumbnailLink,
			})
		}

		if len(photos) == 0 {
			apimodel.Anlogger.Warnf(lc, "get_new_faces.go : get new faces return user [%s] with empty photo list for resolution [%s] for userId [%s]",
				each.UserId, resolution, userId)
			continue
		}

		lastOnlineText, lastOnlineFlag := apimodel.TransformLastOnlineTimeIntoStatusText(userId, each.LastOnlineTime, each.SourceLocale, lc)
		distanceText := apimodel.TransformDistanceInDistanceText(userId, each, lc)

		profiles = append(profiles, commons.Profile{
			UserId:         each.UserId,
			Photos:         photos,
			LastOnlineText: lastOnlineText,
			LastOnlineFlag: lastOnlineFlag,
			DistanceText:   distanceText,
			Age:            each.Age,
			Property:       each.Property,
			Transport:      each.Transport,
			Income:         each.Income,
			Height:         each.Height,
		})

		targetIds = append(targetIds, each.UserId)
	}
	apimodel.Anlogger.Debugf(lc, "get_new_faces.go : prepare [%d] new faces profiles for userId [%s]", len(profiles), userId)

	feedResp.Profiles = profiles

	//to simplify client logic lets remove possible nil objects
	if feedResp.Profiles == nil {
		feedResp.Profiles = make([]commons.Profile, 0)
	}

	apimodel.MarkNewFacesDefaultSort(userId, &feedResp, lc)

	body, err := json.Marshal(feedResp)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : error while marshaling resp [%v] object for userId [%s] : %v", feedResp, userId, err)
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : userId [%s], return %s to client", userId, commons.InternalServerError)
		return commons.NewServiceResponse(commons.InternalServerError), nil
	}

	event := commons.NewProfileWasReturnToNewFacesEvent(userId, sourceIp, targetIds, feedResp.RepeatRequestAfter)
	commons.SendAnalyticEvent(event, userId, apimodel.DeliveryStreamName, apimodel.AwsDeliveryStreamClient, apimodel.Anlogger, lc)
	//commons.SendCloudWatchMetric(apimodel.BaseCloudWatchNamespace, apimodel.NewFaceProfilesReturnMetricName, len(feedResp.Profiles), apimodel.AwsCWClient, apimodel.Anlogger, lc)
	finishTime := commons.UnixTimeInMillis()
	apimodel.Anlogger.Infof(lc, "get_new_faces.go : successfully return repeat request after [%v], [%d] new faces profiles to userId [%s], duration [%v]", feedResp.RepeatRequestAfter, len(feedResp.Profiles), userId, finishTime-startTime)
	apimodel.Anlogger.Debugf(lc, "get_new_faces.go : return successful resp [%s] for userId [%s]", string(body), userId)
	return commons.NewServiceResponse(string(body)), nil
}

//response, repeat request after sec, ok and error string
func getNewFaces(userId string, limit int, lastActionTime int64, resolution string, lc *lambdacontext.LambdaContext) ([]commons.InternalProfiles, int64, bool, string) {

	if limit < 0 {
		limit = newFacesDefaultLimit
	} else if limit > newFacesMaxLimit {
		limit = newFacesMaxLimit
	}
	apimodel.Anlogger.Debugf(lc, "get_new_faces.go : get new faces for userId [%s] with limit [%d]", userId, limit)

	req := commons.InternalGetNewFacesReq{
		UserId:         userId,
		Limit:          limit,
		LastActionTime: lastActionTime,
		Resolution:     resolution,
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

	var response commons.InternalGetNewFacesResp
	err = json.Unmarshal(resp.Payload, &response)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : error unmarshaling response %s into json for userId [%s] : %v", string(resp.Payload), userId, err)
		return nil, 0, false, commons.InternalServerError
	}

	if lastActionTime > response.LastActionTime {
		apimodel.Anlogger.Debugf(lc, "get_new_faces.go : requested lastActionTime [%d] > actual lastActionTime [%d] for userId [%s], diff is [%d]",
			lastActionTime, response.LastActionTime, userId, response.LastActionTime-lastActionTime)
		return nil, apimodel.DefaultRepeatTimeSec, true, ""
	}

	apimodel.Anlogger.Debugf(lc, "get_new_faces.go : successfully got new faces for userId [%s] with limit [%d], resp %v", userId, limit, response)
	return response.NewFaces, 0, true, ""
}

func main() {
	basicLambda.Start(handler)
}
