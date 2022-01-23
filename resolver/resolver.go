package resolver

import (
	"container/list"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	keyReg = regexp.MustCompile(`(\$\{[^\}]+\})`)
)

type resolverFunc struct {
	resolve func(r *Resolver, key string) (string, bool)
}

type element struct {
	Key   string
	Val   *string
	Count int
}

type Resolver struct {
	queue *list.List
	data  map[string]string
}

func (r *Resolver) Add(key string, val *string) {
	item := &element{
		Key: key,
		Val: val,
	}
	r.queue.PushBack(item)
}

func (r *Resolver) AddItem(key string, i int, val *string) {
	item := &element{
		Key: fmt.Sprintf("%s[%d]", key, i),
		Val: val,
	}
	r.queue.PushBack(item)
}

func (r *Resolver) AddList(key string, vals []string) {
	for i := 0; i < len(vals); i++ {
		item := &element{
			Key: fmt.Sprintf("%s[%d]", key, i),
			Val: &vals[i],
		}
		r.queue.PushBack(item)
	}
}

func (r *Resolver) resolve(item *element) (err error) {
	keys := keyReg.FindAllString(*item.Val, -1)
	resolvers := []resolverFunc{
		{resolve: func(r *Resolver, key string) (val string, success bool) {
			val, success = r.data[key]

			return
		}},
		{resolve: func(r *Resolver, key string) (val string, success bool) { return os.LookupEnv(key) }},
	}

	for _, keyFull := range keys {
		key := keyFull[2 : len(keyFull)-1]
		wasSolved := false

		for _, rslvr := range resolvers {
			val, ok := rslvr.resolve(r, key)
			if ok {
				wasSolved = true
				*item.Val = strings.Replace(*item.Val, keyFull, val, 1)

				break
			}
		}

		if !wasSolved {
			fmt.Printf(`resolver: Failed to resolve '%s' in '%s="%s"'`,
				keyFull, item.Key, *item.Val)
			return
		}
	}

	r.data[item.Key] = *item.Val

	return
}

func (r *Resolver) Resolve() (err error) {
	for {
		elem := r.queue.Front()
		if elem == nil {
			return
		}

		item := elem.Value.(*element)
		item.Count += 1

		err = r.resolve(item)
		if err != nil {
			if item.Count > 32 {
				return
			} else {
				err = nil
				r.queue.PushBack(elem.Value)
			}
		}

		r.queue.Remove(elem)
	}
}

func New() (reslv *Resolver) {
	reslv = &Resolver{
		queue: list.New(),
		data:  map[string]string{},
	}

	return
}
