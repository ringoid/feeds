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
)

const (
	newFacesDefaultLimit = 5
	newFacesMaxLimit     = 100
)

func init() {
	apimodel.InitLambdaVars("discover-feed")
}

func handler(ctx context.Context, request events.ALBTargetGroupRequest) (events.ALBTargetGroupResponse, error) {
	lc, _ := lambdacontext.FromContext(ctx)

	start := commons.UnixTimeInMillis()

	userAgent := request.Headers["user-agent"]
	if strings.HasPrefix(userAgent, "ELB-HealthChecker") {
		return commons.NewServiceResponse("{}"), nil
	}

	if request.HTTPMethod != "POST" {
		return commons.NewWrongHttpMethodServiceResponse(), nil
	}
	sourceIp := request.Headers["x-forwarded-for"]

	apimodel.Anlogger.Debugf(lc, "discover.go : start handle request %v", request)

	appVersion, isItAndroid, ok, errStr := commons.ParseAppVersionFromHeaders(request.Headers, apimodel.Anlogger, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "discover.go : return %s to client", errStr)
		return commons.NewServiceResponse(errStr), nil
	}

	reqParam, ok, errStr := parseParams(request.Body, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "discover.go : return %s to client", errStr)
		return commons.NewServiceResponse(errStr), nil
	}

	userId, ok, _, errStr := commons.CallVerifyAccessToken(appVersion, isItAndroid, *reqParam.AccessToken,
		apimodel.InternalAuthFunctionName, apimodel.ClientLambda, apimodel.Anlogger, lc)

	if !ok {
		apimodel.Anlogger.Errorf(lc, "discover.go : return %s to client", errStr)
		return commons.NewServiceResponse(errStr), nil
	}

	reqParam.UserId = &userId

	internalNewFaces, repeatRequestAfter, _, ok, errStr := discover(reqParam, lc)
	if !ok {
		apimodel.Anlogger.Errorf(lc, "discover.go : userId [%s], return %s to client", userId, errStr)
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
			apimodel.Anlogger.Warnf(lc, "discover.go : discover return user [%s] with empty photo list for resolution [%s] for userId [%s]",
				each.UserId, *reqParam.Resolution, userId)
			continue
		}

		lastOnlineText, lastOnlineFlag := apimodel.TransformLastOnlineTimeIntoStatusText(userId, each.LastOnlineTime, each.SourceLocale, lc)
		distanceText := apimodel.TransformDistanceInDistanceText(userId, each, lc)

		profile := commons.Profile{
			UserId:         each.UserId,
			Photos:         photos,
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

		profile = apimodel.CheckProfileBeforeResponse(userId, profile)

		profiles = append(profiles, profile)

		targetIds = append(targetIds, each.UserId)
	}
	apimodel.Anlogger.Debugf(lc, "discover.go : prepare [%d] discover profiles for userId [%s]", len(profiles), userId)

	feedResp.Profiles = profiles

	//to simplify client logic lets remove possible nil objects
	if feedResp.Profiles == nil {
		feedResp.Profiles = make([]commons.Profile, 0)
	}

	apimodel.MarkNewFacesDefaultSort(userId, &feedResp, lc)

	body, err := json.Marshal(feedResp)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "discover.go : error while marshaling resp [%v] object for userId [%s] : %v", feedResp, userId, err)
		apimodel.Anlogger.Errorf(lc, "discover.go : userId [%s], return %s to client", userId, commons.InternalServerError)
		return commons.NewServiceResponse(commons.InternalServerError), nil
	}

	//now check do we need to make new preparation for new faces
	//if howMuchPreparedWeNowHave < commons.NewFacesHardcodedLimit && repeatRequestAfter == 0 {
	//	ok, errStr = prepareNewFacesAsync(userId, lc)
	//	if !ok {
	//		apimodel.Anlogger.Errorf(lc, "get_new_faces.go : userId [%s], return %s to client", userId, errStr)
	//		return commons.NewServiceResponse(errStr), nil
	//	}
	//}
	minA, maxA, maxD := filterString(reqParam)

	execTime := commons.UnixTimeInMillis() - start
	event := commons.NewProfileWasReturnToDiscoverEvent(userId, sourceIp, len(targetIds), minA, maxA, maxD, feedResp.RepeatRequestAfter, execTime)
	commons.SendAnalyticEvent(event, userId, apimodel.DeliveryStreamName, apimodel.AwsDeliveryStreamClient, apimodel.Anlogger, lc)

	apimodel.Anlogger.Infof(lc, "discover.go : successfully return repeat request after [%v], [%d] new faces profiles to userId [%s], minAge [%d], maxAge [%d], maxDistance [%d], duration [%v]",
		feedResp.RepeatRequestAfter, len(feedResp.Profiles), userId, minA, maxA, maxD, execTime)
	//apimodel.Anlogger.Debugf(lc, "discover.go : return successful resp [%s] for userId [%s]", string(body), userId)
	return commons.NewServiceResponse(string(body)), nil
}

func parseParams(params string, lc *lambdacontext.LambdaContext) (*commons.DiscoverRequest, bool, string) {
	//apimodel.Anlogger.Debugf(lc, "discover.go : parse request body %s", params)

	var req commons.DiscoverRequest
	err := json.Unmarshal([]byte(params), &req)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "discover.go : error marshaling required params from the string [%s] : %v", params, err)
		return nil, false, commons.InternalServerError
	}

	if req.AccessToken == nil || len(*req.AccessToken) == 0 {
		apimodel.Anlogger.Errorf(lc, "discover.go : accessToken is empty, request [%v]", req)
		return nil, false, commons.WrongRequestParamsClientError
	}

	if req.Resolution == nil || len(*req.Resolution) == 0 {
		apimodel.Anlogger.Errorf(lc, "discover.go : resolution is empty, request [%v]", req)
		return nil, false, commons.WrongRequestParamsClientError
	}

	if !commons.AllowedPhotoResolution[*req.Resolution] {
		apimodel.Anlogger.Warnf(lc, "discover.go : resolution [%s] is not supported, so use [%s] resolution", *req.Resolution, commons.BiggestDefaultPhotoResolution)
		req.Resolution = &commons.BiggestDefaultPhotoResolution
	}

	if req.LastActionTime == nil || *req.LastActionTime < 0 {
		apimodel.Anlogger.Errorf(lc, "discover.go : lastActionTime is empty or less than zero, request [%v]", req)
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

	if req.Limit != nil {
		if *req.Limit <= 0 {
			lim := newFacesDefaultLimit
			req.Limit = &lim
		} else if *req.Limit > newFacesMaxLimit {
			lim := newFacesMaxLimit
			req.Limit = &lim
		}
	} else {
		lim := newFacesDefaultLimit
		req.Limit = &lim
	}

	//apimodel.Anlogger.Debugf(lc, "discover.go : successfully parse request [%v]", req)
	return &req, true, ""
}

//return minAge, maxAge, maxDistance
func filterString(req *commons.DiscoverRequest) (int, int, int) {
	var minA, maxA, maxD = 18, 56, 150001
	if req.Filter == nil {
		return minA, maxA, maxD
	}

	if req.Filter.MinAge != nil {
		minA = *req.Filter.MinAge
	}

	if req.Filter.MaxAge != nil {
		maxA = *req.Filter.MaxAge
	}

	if req.Filter.MaxDistance != nil {
		maxD = *req.Filter.MaxDistance
	}
	return minA, maxA, maxD
}

func main() {
	basicLambda.Start(handler)
}

//response, repeat request after sec, how much prepared we have now, ok and error string
func discover(request *commons.DiscoverRequest, lc *lambdacontext.LambdaContext) ([]commons.InternalProfiles, int64, int64, bool, string) {

	apimodel.Anlogger.Debugf(lc, "discover.go : discover for userId [%s] with limit [%d]", *request.UserId, *request.Limit)

	jsonBody, err := json.Marshal(request)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "discover.go : error marshaling req %s into json for userId [%s] : %v", request, *request.UserId, err)
		return nil, 0, 0, false, commons.InternalServerError
	}

	resp, err := apimodel.ClientLambda.Invoke(&lambda.InvokeInput{FunctionName: aws.String(apimodel.DiscoverFunctionName), Payload: jsonBody})
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "discover.go : error invoke function [%s] with body %s for userId [%s] : %v", apimodel.DiscoverFunctionName, jsonBody, *request.UserId, err)
		return nil, 0, 0, false, commons.InternalServerError
	}

	if *resp.StatusCode != 200 {
		apimodel.Anlogger.Errorf(lc, "discover.go : status code = %d, response body %s for request %s, for userId [%s] ", *resp.StatusCode, string(resp.Payload), jsonBody, *request.UserId)
		return nil, 0, 0, false, commons.InternalServerError
	}

	var response commons.InternalGetNewFacesResp
	err = json.Unmarshal(resp.Payload, &response)
	if err != nil {
		apimodel.Anlogger.Errorf(lc, "discover.go : error unmarshaling response %s into json for userId [%s] : %v", string(resp.Payload), *request.UserId, err)
		return nil, 0, 0, false, commons.InternalServerError
	}

	if *request.LastActionTime > response.LastActionTime {
		apimodel.Anlogger.Debugf(lc, "discover.go : requested lastActionTime [%d] > actual lastActionTime [%d] for userId [%s], diff is [%d]",
			*request.LastActionTime, response.LastActionTime, *request.UserId, response.LastActionTime - *request.LastActionTime)
		return nil, apimodel.DefaultRepeatTimeSec, 0, true, ""
	}

	//apimodel.Anlogger.Debugf(lc, "discover.go : successfully got profiles for userId [%s] with limit [%d], resp %v", *request.UserId, *request.Limit, response)
	return response.NewFaces, 0, response.HowMuchPrepared, true, ""
}
