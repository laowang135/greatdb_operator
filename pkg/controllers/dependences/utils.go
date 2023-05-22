package dependences

import "time"

// Returns the exponential level time of 2 based on num
func GetExponentialLevelDelay(num int) time.Duration {

	initTime := 4 // 4s
	exp := 2
	for i := 0; i < num%5; i++ {
		exp *= 2
	}
	return time.Second * time.Duration(initTime*exp)

}
