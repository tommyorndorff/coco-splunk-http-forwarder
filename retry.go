package main

import (
	"log"
	"math"
	"sync"
	"time"
)

const (
	sleepTime  = 100
	maxBackoff = 9
	minBackoff = 2
)

type Retry interface {
	Start()
	Enqueue(s string) error
	Dequeue() ([]string, error)
}

type serviceStatus struct {
	sync.Mutex
	healthy   bool
	timestamp time.Time
}

func (status *serviceStatus) setHealthy(healthy bool, time time.Time) {
	status.Lock()
	defer status.Unlock()
	status.healthy = healthy
	status.timestamp = time
}

func (status *serviceStatus) isHealthy() bool {
	status.Lock()
	defer status.Unlock()
	return status.healthy
}

type retry struct {
	action        func(string) error
	statusChecker func() *serviceStatus
	cache         S3Service
}

func NewRetry(action func(string) error, statusChecker func() *serviceStatus, bucketName string, awsRegion string) Retry {
	svc, _ := NewS3Service(bucketName, awsRegion)
	return retry{action, statusChecker, svc}
}

func (logRetry retry) Start() {
	go func() {
		level := 3 // start conservative
		for {
			status := logRetry.statusChecker()
			if status.isHealthy() {
				entries, err := logRetry.Dequeue()
				if err != nil {
					log.Printf("Failure retrieving logs from S3 %v\n", err)
				} else if len(entries) > 0 {
					log.Printf("Read %v messages from S3\n", len(entries))
				}
				for _, entry := range entries {
					err := logRetry.action(entry)
					if err != nil {
						if level < maxBackoff {
							level++
						}
					} else {
						if level > minBackoff {
							level--
						}
					}
					sleepDuration := time.Duration((0.15*math.Pow(2, float64(level))-0.2)*1000) * time.Millisecond
					if err != nil {
						log.Printf("Retried one message unsuccessfully, ")
					} else {
						log.Printf("Retried one message successfully, ")
					}
					log.Printf("sleeping for %v\n", sleepDuration)
					time.Sleep(sleepDuration)
				}
			}
			time.Sleep(sleepTime * time.Millisecond)
		}
	}()
}

func (logRetry retry) Enqueue(s string) error {
	return logRetry.cache.Put(s)
}

func (logRetry retry) Dequeue() ([]string, error) {
	return logRetry.cache.ListAndDelete()
}
