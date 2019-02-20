package apimodel

import (
	"fmt"
	"github.com/ringoid/commons"
)

const (
	DefaultRepeatTimeSec = int64(800)
)

type InternalGetNewFacesReq struct {
	UserId         string `json:"userId"`
	Limit          int    `json:"limit"`
	LastActionTime int64  `json:"requestedLastActionTime"`
}

func (req InternalGetNewFacesReq) String() string {
	return fmt.Sprintf("%#v", req)
}

type InternalGetNewFacesResp struct {
	NewFaces       []InternalNewFace `json:"newFaces"`
	LastActionTime int64             `json:"lastActionTime"`
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
	Profiles           []commons.Profile `json:"profiles"`
	RepeatRequestAfter int64             `json:"repeatRequestAfter"`
}

func (resp GetNewFacesFeedResp) String() string {
	return fmt.Sprintf("%#v", resp)
}

//Likes You

type InternalLMMReq struct {
	UserId                  string `json:"userId"`
	RequestNewPart          bool   `json:"requestNewPart"`
	RequestedLastActionTime int64  `json:"requestedLastActionTime"`
}

func (req InternalLMMReq) String() string {
	return fmt.Sprintf("%#v", req)
}

type InternalLMMResp struct {
	Profiles       []InternalProfiles `json:"profiles"`
	LastActionTime int64              `json:"lastActionTime"`
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
	LikesYou           []commons.Profile `json:"likesYou"`
	Matches            []commons.Profile `json:"matches"`
	Messages           []commons.Profile `json:"messages"`
	RepeatRequestAfter int64             `json:"repeatRequestAfter"`
}

func (resp LMMFeedResp) String() string {
	return fmt.Sprintf("%#v", resp)
}
