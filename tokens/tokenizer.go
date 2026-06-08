package tokens

import (
	"sync"
)

type Token = uint32

var toTokenCache = make(map[string][]Token, 1024)
var wordToToken = make(map[string]Token, 1024)
var lock = sync.RWMutex{}

func CacheSize() int {
	lock.RLock()
	size := len(toTokenCache)
	lock.RUnlock()

	return size
}

// ToTokens converts a string into tokens.
// Allownewtokens is a flag that determines whether new tokens should be created for words that are not already in the tokenizer's wordToToken map.
// If allownewtokens = false and a token is not found, the function returns nil.
// If an empty string is passed, the function will return nil.
func ToTokens(in string) []Token {
	lock.RLock()
	cacheEntry, ok := toTokenCache[in]
	lock.RUnlock()
	if ok {
		return cacheEntry
	}

	lock.Lock()
	defer lock.Unlock()

	var resp []Token
	for word := range sanitizedWords(in) {
		token, ok := wordToToken[word]
		if ok {
			resp = append(resp, token)
		} else {
			token = Token(len(wordToToken))
			wordToToken[word] = token
			resp = append(resp, token)
		}
	}

	addToTokenCacheEntry(in, resp)
	return resp
}

func addToTokenCacheEntry(key string, value []Token) {
	if len(toTokenCache) > 100_000 {
		// Delete a random entry from the cache
		for key := range toTokenCache {
			delete(toTokenCache, key)
			break
		}
	}

	toTokenCache[key] = value
}

func CleanupCache() {
	lock.Lock()
	toTokenCache = map[string][]Token{}
	lock.Unlock()
}
