package language

// ShortCode contains the byte version of the 2 character ISO 639-1 language code.
type ShortCode [2]byte

func (sc ShortCode) IsZero() bool {
	return sc == ShortCode{}
}

func (sc ShortCode) String() string {
	if sc.IsZero() {
		return ""
	}
	return string(sc[:])
}
