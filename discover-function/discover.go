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

	userAgent := request.Headers["user-agent"]
	if strings.HasPrefix(userAgent, "ELB-HealthChecker") {
		return commons.NewServiceResponse("{}"), nil
	}

	if request.HTTPMethod != "POST" {
		return commons.NewWrongHttpMethodServiceResponse(), nil
	}
	//sourceIp := request.Headers["x-forwarded-for"]

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

	return commons.NewServiceResponse(userId), nil
}

func parseParams(params string, lc *lambdacontext.LambdaContext) (*apimodel.DiscoverRequest, bool, string) {
	apimodel.Anlogger.Debugf(lc, "discover.go : parse request body %s", params)

	var req apimodel.DiscoverRequest
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

	apimodel.Anlogger.Debugf(lc, "discover.go : successfully parse request [%v]", req)
	return &req, true, ""
}

func main() {
	basicLambda.Start(handler)
}
