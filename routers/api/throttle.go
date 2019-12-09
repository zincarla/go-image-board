package api

import (
	"sync"
	"time"
)

//ThrottleMap is an in-memory cache of the API throttle times
type ThrottleMap struct {
	timeMap   map[uint64]time.Time
	timeMutex sync.Mutex
}

//Throttle is a thread-safe cache of API throttle times
var Throttle ThrottleMap

//Init creates the internal map, must be called before using
func (apiThrottle *ThrottleMap) Init() {
	apiThrottle.timeMutex.Lock()
	defer apiThrottle.timeMutex.Unlock()
	apiThrottle.timeMap = make(map[uint64]time.Time)
}

//SetValue sets the throttle timeout for a given user in milliseconds
func (apiThrottle *ThrottleMap) SetValue(UserID uint64, Timeout int64) {
	apiThrottle.timeMutex.Lock()
	defer apiThrottle.timeMutex.Unlock()
	apiThrottle.timeMap[UserID] = time.Now().Add(time.Millisecond * time.Duration(Timeout))
}

//GetValue returns the throttle time for a given user, returns zero time if user not found
func (apiThrottle *ThrottleMap) GetValue(UserID uint64) time.Time {
	apiThrottle.timeMutex.Lock()
	defer apiThrottle.timeMutex.Unlock()
	if _, ok := apiThrottle.timeMap[UserID]; ok {
		return apiThrottle.timeMap[UserID]
	}
	return time.Time{}
}

//CanUseAPI returns a boolean of whether the specified user should be allowed to make an API call
func (apiThrottle *ThrottleMap) CanUseAPI(UserID uint64) (bool, time.Time) {
	apiThrottle.timeMutex.Lock()
	defer apiThrottle.timeMutex.Unlock()
	if value, ok := apiThrottle.timeMap[UserID]; ok {
		return value.Before(time.Now()), value
	}
	return true, time.Time{} //No entry for user so gtg
}

//DeleteValue removes an entry from the time map
func (apiThrottle *ThrottleMap) DeleteValue(UserID uint64) {
	apiThrottle.timeMutex.Lock()
	defer apiThrottle.timeMutex.Unlock()
	delete(apiThrottle.timeMap, UserID)
}
