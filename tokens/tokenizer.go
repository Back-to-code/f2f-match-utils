package tokens

import (
	"sync"
)

type Token = uint32

var toTokenCache = make(map[string][][]Token, 1024)
var wordToToken = make(map[string]Token, 1024)
var lock = sync.RWMutex{}

func CacheSize() int {
	lock.RLock()
	size := len(toTokenCache)
	lock.RUnlock()

	return size
}

// ToTokens converts a string into unique tokens.
// Every word is converted to a unique token.
// Some inputs have multiple ways to convert the input to tokens, for those cases we return multiple responses.
// The outer slice are the total variants (mainly 1), the inner slice is the input string as a list of tokens
// When you want the strictest token variant use the first response (resp[0]) but do note that resp might be a empty slice
// Note "" will produce [] and not [[]]!
func ToTokens(in string) [][]Token {
	lock.RLock()
	cacheEntry, ok := toTokenCache[in]
	lock.RUnlock()
	if ok {
		return cacheEntry
	}

	lock.Lock()
	defer lock.Unlock()

	resp := [][]Token{{}}
	for parts := range sanitizedWords(in) {
		if parts.addition != nil {
			if len(resp) == 1 {
				// Branch the response into multiple responses
				newTokens := make([]Token, len(resp[0]))
				copy(newTokens, resp[0])
				resp = append(resp, newTokens)

				for _, word := range []string{
					parts.base,
					*parts.addition,
				} {
					token, ok := wordToToken[word]
					if ok {
						resp[1] = append(resp[1], token)
					} else {
						token = Token(len(wordToToken))
						wordToToken[word] = token
						resp[1] = append(resp[1], token)
					}
				}
			}
		}

		if parts.addition != nil {
			parts.base += *parts.addition
		}

		token, ok := wordToToken[parts.base]
		if ok {
			resp[0] = append(resp[0], token)
		} else {
			token = Token(len(wordToToken))
			wordToToken[parts.base] = token
			resp[0] = append(resp[0], token)
		}
	}

	for idx := len(resp) - 1; idx >= 0; idx-- {
		// Filter out the empty branches, including branch 0 when no tokens were
		// produced at all (e.g. empty input), so the result is [] and never [[]].
		if len(resp[idx]) == 0 {
			resp = append(resp[:idx], resp[idx+1:]...)
		}
	}

	addToTokenCacheEntry(in, resp)
	return resp
}

func addToTokenCacheEntry(key string, value [][]Token) {
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
	toTokenCache = map[string][][]Token{}
	lock.Unlock()
}
