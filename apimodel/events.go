package apimodel

import (
	"time"
	"fmt"
)

type ProfileWasReturnToNewFacesEvent struct {
	UserId              string   `json:"userId"`
	TargetUserIds       []string `json:"targetUserIds"`
	TimeToDeleteViewRel int64    `json:"timeToDelete"`
	SeenProfilesNum     int      `json:"seenProfilesNum"`
	UnixTime            int64    `json:"unixTime"`
	EventType           string   `json:"eventType"`
}

func (event ProfileWasReturnToNewFacesEvent) String() string {
	return fmt.Sprintf("%#v", event)
}

func NewProfileWasReturnToNewFacesEvent(userId string, timeToDeleteViewRel int64, targetIds []string) ProfileWasReturnToNewFacesEvent {
	return ProfileWasReturnToNewFacesEvent{
		UserId:              userId,
		TargetUserIds:       targetIds,
		TimeToDeleteViewRel: timeToDeleteViewRel,
		SeenProfilesNum:     len(targetIds),
		UnixTime:            time.Now().Unix(),
		EventType:           "FEEDS_NEW_FACES_SEEN_PROFILES",
	}
}
