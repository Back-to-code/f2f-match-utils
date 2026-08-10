package sanitize

import "unicode/utf8"

// NormalizeString normalizes the input.
// It changes the following things:
// - Space like characters are converted to spaces
// - Duplicated spaces are removed
// - Spaces around the string are removed
// - Non number and non letter characters are removed
// - Uppercase characters are converted to lowercase
// - Unicode characters are converted to their ascii equivalent (when possible)
func NormalizeString(inStr string) string {
	if len(inStr) == 0 {
		return ""
	}

	inRunes := []rune(inStr)

	for idx := len(inRunes) - 1; idx >= 0; idx-- {
		c := inRunes[idx]
		if c >= '0' && c <= '9' || c >= 'a' && c <= 'z' {
			continue
		}

		if c >= utf8.RuneSelf {
			var didNothing bool
			c, didNothing = checkAndCorrectUnicodeChar(c)
			if didNothing {
				// This is just a utf8 character that we don't have to do anything with
				continue
			}
			// Write the corrected character back to the rune slice
			inRunes[idx] = c
			if c != ' ' {
				continue
			}
		}

		switch c {
		case '\n', '\r', '\t', ' ':
			if idx == len(inRunes)-1 {
				// Trim the space like characters around the input
				inRunes = inRunes[:idx]
				continue
			} else if idx != 0 {
				cToTheLeft, _ := checkAndCorrectUnicodeChar(inRunes[idx-1])
				if cToTheLeft == ' ' || cToTheLeft == '\n' || cToTheLeft == '\r' || cToTheLeft == '\t' {
					// The character to the left is also a whitespace character, so we can remove this char
					// By doing this we remove the duplicated spaces
					inRunes = append(inRunes[:idx], inRunes[idx+1:]...)
					continue
				}
			}

			if c != ' ' {
				inRunes[idx] = ' '
			} else if idx == 0 {
				// The first character is a space, trim the front
				// We don't have to worry if the next character where spaces because
				// they would be already removed by a previous if else check
				inRunes = inRunes[1:]
			}
		default:
			if c >= 'A' && c <= 'Z' {
				// Convert uppercase to lowercase
				inRunes[idx] += 'a' - 'A'
			} else if c < utf8.RuneSelf {
				// Remove all other special ascii characters
				inRunes = append(inRunes[:idx], inRunes[idx+1:]...)
			}
		}
	}

	return string(inRunes)
}

// checkAndCorrectUnicodeChar checks if the rune is a unicode char and corrects it
func checkAndCorrectUnicodeChar(c rune) (cOrFixed rune, didNothing bool) {
	switch c {
	case 'à', 'À', 'á', 'Á', 'â', 'Â', 'ã', 'Ã', 'ä', 'Ä', 'å', 'Å', 'æ', 'Æ':
		return 'a', false
	case 'è', 'È', 'é', 'É', 'ê', 'Ê', 'ë', 'Ë':
		return 'e', false
	case 'ì', 'Ì', 'í', 'Í', 'î', 'Î', 'ï', 'Ï':
		return 'i', false
	case 'ò', 'Ò', 'ó', 'Ó', 'ô', 'Ô', 'õ', 'Õ', 'ö', 'Ö', 'ð', 'Ð', 'ø', 'Ø':
		return 'o', false
	case 'ù', 'Ù', 'ú', 'Ú', 'û', 'Û', 'ü', 'Ü':
		return 'u', false
	case 'ß':
		return 's', false
	case 'ñ', 'Ñ':
		return 'n', false
	case 'ý', 'Ý', 'ÿ', 'Ÿ':
		return 'y', false
	case 'ç', 'Ç':
		return 'c', false
	case '©', '®',
		'¿',
		'¡',
		// NFD Accent characters
		// For more information about these see: https://en.wikipedia.org/wiki/Unicode_equivalence
		768, // the accent only of: à
		769, // the accent only of: á
		770, // the accent only of: â
		771, // the accent only of: ã
		776, // the accent only of: ä
		778, // the accent only of: å
		// Sussy spaces
		0x00A0, // Non-breaking space
		0x2002, // En space
		0x2003, // Em space
		0x2004, // Three-per-em space
		0x2005, // Four-per-em space
		0x2006, // Six-per-em space
		0x2007, // Figure space
		0x2008, // Punctuation space
		0x2009, // Thin space
		0x200A, // Hair space
		0x200B, // Zero width space
		0x202F, // Narrow no-break space
		0x205F, // Medium mathematical space
		0x3000, // Ideographic space
		'\u201C',
		'\u201D',
		'\u2018',
		'‵',
		'‹',
		'›',
		'»',
		'«',
		utf8.RuneError:
		return ' ', false
	default:
		return c, true
	}
}
