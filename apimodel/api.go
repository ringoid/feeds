package apimodel

import (
	"fmt"
	"github.com/ringoid/commons"
)

type InternalGetNewFacesReq struct {
	UserId string `json:"userId"`
	Limit  int    `json:"limit"`
}

func (req InternalGetNewFacesReq) String() string {
	return fmt.Sprintf("%#v", req)
}

type InternalGetNewFacesResp struct {
	NewFaces []InternalNewFace `json:"newFaces"`
}

type InternalNewFace struct {
	UserId   string   `json:"userId"`
	PhotoIds []string `json:"photoIds"`
}

func (resp InternalGetNewFacesResp) String() string {
	return fmt.Sprintf("%#v", resp)
}

type GetNewFacesFeedResp struct {
	commons.BaseResponse
	Profiles []commons.Profile `json:"profiles"`
}

func (resp GetNewFacesFeedResp) String() string {
	return fmt.Sprintf("%#v", resp)
}

//Likes You

type InternalLMMReq struct {
	UserId                  string `json:"userId"`
	RequestNewPart          bool   `json:"requestNewPart"`
	RequestedLastActionTime int    `json:"requestedLastActionTime"`
}

func (req InternalLMMReq) String() string {
	return fmt.Sprintf("%#v", req)
}

type InternalLMMResp struct {
	Profiles       []InternalProfiles `json:"profiles"`
	LastActionTime int                `json:"lastActionTime"`
}

type InternalProfiles struct {
	UserId   string   `json:"userId"`
	PhotoIds []string `json:"photoIds"`
}

func (resp InternalLMMResp) String() string {
	return fmt.Sprintf("%#v", resp)
}

type LMMFeedResp struct {
	commons.BaseResponse
	LikesYou              []commons.Profile `json:"likesYou"`
	Matches               []commons.Profile `json:"matches"`
	RepeatRequestAfterSec int               `json:"repeatRequestAfterSec"`
}

func (resp LMMFeedResp) String() string {
	return fmt.Sprintf("%#v", resp)
}
