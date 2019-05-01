package apimodel

import (
	"fmt"
	"github.com/ringoid/commons"
)

const (
	DefaultRepeatTimeSec = int64(800)
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
