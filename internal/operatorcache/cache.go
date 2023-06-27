package operatorcache

import (
	"github.com/patrickmn/go-cache"
	"time"
)

var myCache = cache.New(5*time.Minute, 150*time.Second)

func Get[T any](objectName string) (T, bool) {
	return myCache.Get(objectName)
}

func AddOrReplace[T any](name string, val T, duration time.Duration) {
	myCache.Set(name, val, duration)
}
