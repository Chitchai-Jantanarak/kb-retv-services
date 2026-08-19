package omnichannel

import (
	"fmt"
	"hash/fnv"
	"sync"
)

var convLocks [64]sync.Mutex

func conversationLock(companyID, accountID int64, customer string) *sync.Mutex {
	h := fnv.New32a()
	fmt.Fprintf(h, "%d|%d|%s", companyID, accountID, customer)
	return &convLocks[h.Sum32()%uint32(len(convLocks))]
}
