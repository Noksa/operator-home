package operatorcache

import (
	"github.com/patrickmn/go-cache"
	"time"
)

var myCache = cache.New(5*time.Minute, 150*time.Second)

func Get[T any](objectName string) (T, bool) {
	val, _ := myCache.Get(objectName)
	realVal, ok := val.(T)
	return realVal, ok
}

func AddOrReplace[T any](name string, val T, duration time.Duration) {
	myCache.Set(name, val, duration)
}
