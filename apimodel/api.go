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

type Profile struct {
	UserId string  `json:"userId"`
	Photos []Photo `json:"photos"`
}

type Photo struct {
	PhotoId  string `json:"photoId"`
	PhotoUri string `json:"photoUri"`
}

type ProfilesResp struct {
	commons.BaseResponse
	WarmUpRequest bool      `json:"warmUpRequest"`
	Profiles      []Profile `json:"profiles"`
}

func (resp ProfilesResp) String() string {
	return fmt.Sprintf("%#v", resp)
}

type GetNewFacesFeedResp struct {
	commons.BaseResponse
	Profiles []Profile `json:"profiles"`
}

func (resp GetNewFacesFeedResp) String() string {
	return fmt.Sprintf("%#v", resp)
}

type FacesWithUrlResp struct {
	//contains userId_photoId like a key and photoUrl like a value
	UserIdPhotoIdKeyUrlMap map[string]string `json:"urlPhotoMap"`
}

func (resp FacesWithUrlResp) String() string {
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
	LikesYouNewProfiles   []Profile `json:"likesYouNewProfiles"`
	LikesYouOldProfiles   []Profile `json:"likesYouOldProfiles"`
	MatchesNewProfiles    []Profile `json:"matchesNewProfiles"`
	MatchesOldProfiles    []Profile `json:"matchesOldProfiles"`
	RepeatRequestAfterSec int       `json:"repeatRequestAfterSec"`
}

func (resp LMMFeedResp) String() string {
	return fmt.Sprintf("%#v", resp)
}
