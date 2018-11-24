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

type GetNewFacesResp struct {
	commons.BaseResponse
	WarmUpRequest bool      `json:"warmUpRequest"`
	Profiles      []Profile `json:"profiles"`
}

func (resp GetNewFacesResp) String() string {
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
