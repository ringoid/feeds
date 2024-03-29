package apimodel

import (
	"fmt"
	"github.com/ringoid/commons"
)

const (
	DefaultRepeatTimeSec     = int64(800)
	DefaultPoolRepeatTimeSec = int64(3000)
	IsDebugLogEnabled        = false
)

type GetNewFacesFeedResp struct {
	commons.BaseResponse
	Profiles           []commons.Profile `json:"profiles"`
	RepeatRequestAfter int64             `json:"repeatRequestAfter"`
}

func (resp GetNewFacesFeedResp) String() string {
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

type GetLcFeedResp struct {
	commons.BaseResponse
	RepeatRequestAfter     int64             `json:"repeatRequestAfter"`
	AllLikesYouProfilesNum int               `json:"allLikesYouProfilesNum"`
	AllMessagesProfilesNum int               `json:"allMessagesProfilesNum"`
	LikesYou               []commons.Profile `json:"likesYou"`
	Messages               []commons.Profile `json:"messages"`
}

func (resp GetLcFeedResp) String() string {
	return fmt.Sprintf("%#v", resp)
}

type LMHISFeedResp struct {
	commons.BaseResponse
	LikesYou           []commons.Profile `json:"likesYou"`
	Matches            []commons.Profile `json:"matches"`
	Hellos             []commons.Profile `json:"hellos"`
	Inbox              []commons.Profile `json:"inbox"`
	Sent               []commons.Profile `json:"sent"`
	RepeatRequestAfter int64             `json:"repeatRequestAfter"`
}

func (resp LMHISFeedResp) String() string {
	return fmt.Sprintf("%#v", resp)
}

type ChatFeedResponse struct {
	commons.BaseResponse
	ProfileChat        commons.Profile `json:"chat"`
	IsChatExists       bool            `json:"chatExists"`
	RepeatRequestAfter int64           `json:"repeatRequestAfter"`
	PullAgainAfter     int64           `json:"pullAgainAfter"`
}

func (resp ChatFeedResponse) String() string {
	return fmt.Sprintf("%#v", resp)
}
