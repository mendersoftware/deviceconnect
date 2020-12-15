// Copyright 2020 Northern.tech AS
//
//    All Rights Reserved

package utils

import "time"

// Clock interface
type Clock interface {
	Now() time.Time
}

// RealClock provides a real clock
type RealClock struct{}

// Now returns the current date and time
func (RealClock) Now() time.Time {
	return time.Now()
}
