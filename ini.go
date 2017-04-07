package main

import (
	"bufio"
	"errors"
	. "fmt"
	"io"
	"os"
	"strings"
)

//Represent a key-value
type tagOptions struct {
	next  *tagOptions //next option
	name  string      //key
	value string      //value
}

//Represent a section
type tagSections struct {
	options *tagOptions  //global options(section name is blank)
	next    *tagSections //next section
	name    string       //the name of the section
}

var (
	gSections    tagSections
	gLastSection *tagSections
	gLastOption  *tagOptions
	keyName      string
	buffer       string
	meetSection  bool
	line         int

	ErrParserError = errors.New("Parser error")
	ErrMalFormed   = errors.New("Input is malformed, ends inside comment or literal")
	ErrKeyNotFound = errors.New("key not found")
)

//FSM ACTION FUNCTIONS --  BEGIN
func parseError(state int, symbol rune) error {
	return ErrParserError
}

func newSection(state int, symbol rune) error {
	if symbol != '[' {
		buffer += string(symbol)
	}

	meetSection = true
	return nil
}

func endSection(state int, symbol rune) error {
	var cur *tagSections

	sec := new(tagSections)

	if gSections.next == nil { /* no section yet */
		gSections.next = sec
	} else {
		cur = &gSections
		for cur.next != nil {
			cur = cur.next
		}
		cur.next = sec
	}

	gLastSection = sec
	gLastSection.name = buffer
	buffer = ""

	return nil
}

func newOption(state int, symbol rune) error {
	keyName += string(symbol)

	return nil
}

func newValue(state int, symbol rune) error {
	if keyName != "" {
		option := new(tagOptions)
		if meetSection { /* already meet section(s) */
			if gLastSection.options == nil {
				gLastSection.options = option
			} else {
				gLastOption.next = option
			}
		} else {
			gSections.name = ""           //global section's name is empty
			if gSections.options == nil { /* no option yet */
				gSections.options = option
			} else {
				cur := gSections.options
				for cur.next != nil {
					cur = cur.next
				}
				cur.next = option
			}
		}
		gLastOption = option

		gLastOption.name = keyName
		keyName = ""
	}
	if symbol != '=' {
		gLastOption.value += string(symbol)
	}

	/* InStr state, if string contains multiple lines. */
	if symbol == '\n' {
		line++
	}

	return nil
}

func newLine(state int, symbol rune) error {
	line++
	return nil
}

//FSM ACTION FUNCTIONS --  END

func clearInnerData() {
	gSections.options = nil
	gSections.next = nil
	gSections.name = ""

	if gLastSection != nil {
		if gLastSection.options != nil {
			gLastSection.options = nil
		}
		gLastSection.next = nil
		gLastSection.name = ""
	}

	if gLastOption != nil {
		if gLastOption.next != nil {
			gLastOption.next = nil
		}
		gLastOption.name = ""
		gLastOption.value = ""
	}

	keyName, buffer = "", ""
	meetSection, line = false, 1
}

func CfgParseFile(filename string) error {
	fin, err := os.Open(filename)
	if err != nil {
	    return err
	}
	defer fin.Close()

	return CfgParse(fin)
}

func CfgParse(r io.Reader) error {
	//Rules for describing the state change and associated actions for the FSM
	fsm := []struct {
		state    int  //current state
		c        rune //current rune read
		newState int  //next state
		//FSM action to execute for a certain state
		action func(state int, symbol rune) error
	}{

		/* state = [InOptions] - 0*/
		/* key name could not have space */
		{0, '\n', 0, newLine},
		{0, '\r', 7, nil},
		{0, ' ', 0, nil},  /* skip whitespace */
		{0, '\t', 0, nil}, /* skip whitespace */
		{0, ';', 1, nil},
		{0, '#', 1, nil},
		{0, '=', 4, newValue},
		{0, '[', 2, newSection},
		{0, 0, 0, newOption},

		/* state = [Comment] - 1 */
		{1, '\n', 0, newLine},
		{1, '\r', 7, nil},
		{1, 0, 1, nil},

		/* state = [InSection] - 2 */
		/* section name could have space */
		{2, ']', 3, endSection},
		//rule{2, ';', 8, nil},      /* section could not contail ';' */
		//rule{2, '#', 8, nil},      /* section could not contail '#' */
		{2, 0, 2, newSection}, /* section could contail ';' or '#' */

		/* state = [EndSection] - 3 */
		{3, ';', 1, nil},
		{3, '#', 1, nil},
		{3, '\n', 0, newLine},
		{3, '\r', 7, nil},
		{3, 0, 3, nil}, //? "[xxx] yyyy", then yyyy will be ignored

		/* state = [InValue] - 4 */
		{4, '#', 1, nil},
		{4, ';', 1, nil},
		{4, '"', 5, nil},
		{4, ' ', 4, nil},  /* skip whitespace */
		{4, '\t', 4, nil}, /* skip whitespace */
		{4, '\n', 0, newLine},
		{4, '\r', 7, nil},
		{4, 0, 4, newValue},

		/* state = [InStr] - 5 */
		/* Note: when string value accross multiple lines,
		 * then 'line report is not correct.
		 */
		{5, '\\', 6, nil},
		{5, '"', 4, nil},
		{5, 0, 5, newValue},

		/* state = [StrQuote] - 6 */
		{6, 0, 5, newValue},

		/* state = [CHECKLINE] - 7 */
		{7, '\n', 0, newLine},
		{7, 0,    0, newLine},

		/* state = [Invalid] - 8 */
		{8, 0, -1, parseError},
	}

	clearInnerData()

	state := 0
	line = 1

	reader := bufio.NewReader(r)

	var ch rune
	var err error
	for {
		ch, _, err = reader.ReadRune()
		if err != nil {
			break
		}

		for i := 0; i < len(fsm); i++ {
			currFsm := fsm[i]
			if currFsm.state == state && (currFsm.c == ch || currFsm.c == 0) {
				/* state action */
				if currFsm.action != nil {
					if actionErr := currFsm.action(state, ch); actionErr != nil {
						return actionErr
					}
				}
				state = currFsm.newState
				break
			} //end if
		} //inner for
	} //outer for

	if state != 0 {
		Printf("state is %d\n", state)
		return ErrMalFormed
	}

	return nil
}

/* for debug only */
func CfgPrint() {
	var opt *tagOptions
	var sec *tagSections

	Println("\n==================RESULT:GLOBAL==================")

	if len(gSections.name) == 0 { /* global option */
		opt = gSections.options

		for opt != nil {
			Printf("options key[%v], value=[%v]\n", opt.name, opt.value)
			opt = opt.next
		} //end for
	}

	Printf("\n\n==================RESULT:SECTIONS==================")
	sec = gSections.next
	for sec != nil {
		Printf("\nsection name=[%v]\n", sec.name)

		opt = sec.options
		for opt != nil {
			Printf("\toptions key[%v], value=[%v]\n", opt.name, opt.value)
			opt = opt.next
		} //end inner for

		sec = sec.next
	} //end for
}

func CfgGet(section, key string) (out string, err error) {
	var opt *tagOptions
	var sec *tagSections

	out = ""

	if len(section) == 0 { /* global options */
		opt = gSections.options

		for opt != nil {
			if opt.name == key {
				//out = opt.value[0:1] + opt.value[1:] //copy string
				out = opt.value
				return
			}
			opt = opt.next
		} //end for

		err = ErrKeyNotFound
		return
	}

	sec = gSections.next
	for sec != nil {
		if sec.name == section {
			opt = sec.options
			for opt != nil {
				if opt.name == key {
					//out = opt.value[0:1] + opt.value[1:] //copy string
					out = opt.value
					return
				}

				opt = opt.next
			} //end inner for
		}

		sec = sec.next
	} //end for

	return
}

func main() {

	var out string
	var err error

	//open file for reading
	//fin, _ := os.Open("./test.ini")
	//defer fin.Close()

	//if err = CfgParse(fin); err != nil {
	//	Printf("Error:%s", err.Error())
	//	return
	//}

	//read from string
	const testData = `
	server = "the forgotten server"
	
	[test_data]
	
	author = "raggaer"
	author_email = "xxx@gmail.com"
	author_age = 20
	`

	if err = CfgParse(strings.NewReader(testData)); err != nil {
	//if err = CfgParse(bytes.NewBufferString(testData)); err != nil {  //OK, needs to import "bytes"
		Printf("Error:%s", err.Error())
	}
	CfgPrint()
	Println("\n\n\n\n")



	if err = CfgParseFile("./test.ini"); err != nil {
		Printf("Error:%s", err.Error())
		return
	}

	Println("\n==================GET RESULT:GLOBAL==================")
	out, _ = CfgGet("", "aa")
	Printf("Global section, aa=[%v]\n", out)

	out, _ = CfgGet("", "a")
	Printf("Global section, a=[%v]\n", out)

	out, _ = CfgGet("", "b")
	Printf("Global section, b=[%v]\n", out)

	out, _ = CfgGet("", "c")
	Printf("Global section, c=[%v]\n", out)

	Println("\n==================GET RESULT:[ab;cdefg]==================")
	out, _ = CfgGet("ab;cdefg", "c")
	Printf("Named section[ab;cdefg], c=[%v]\n", out)

	out, _ = CfgGet("ab;cdefg", "d")
	Printf("Named section[ab;cdefg], d=[%v]\n", out)

	out, _ = CfgGet("ab;cdefg", "e")
	Printf("Named section[ab;cdefg], e=[%v]\n", out)

	Println("\n==================GET RESULT:[xxxx]==================")
	out, _ = CfgGet("xxxx", "e")
	Printf("Named section[xxxx], e=[%v]\n", out)

	out, _ = CfgGet("xxxx", "m")
	Printf("Named section[xxxx], m=[%v]\n", out)

	out, _ = CfgGet("xxxx", "n")
	Printf("Named section[xxxx], n=[%v]\n", out)

	Println("\n==================[DEBUG]==================\n")
	CfgPrint()
}
