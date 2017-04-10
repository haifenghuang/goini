package main

import (
	"bufio"
	"errors"
	. "fmt"
	"io"
	"bytes"
    "io/ioutil"
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

type Config struct {
	*config
}

type config struct {
	reader      *bufio.Reader
	sections    tagSections
	lastSection *tagSections
	lastOption  *tagOptions
	keyName     string
	buffer      string
	meetSection bool
	line        int
}

var (
	ErrParserError = errors.New("Parser error")
	ErrMalFormed   = errors.New("Input is malformed, ends inside comment or literal")
	ErrKeyNotFound = errors.New("key not found")
)

//FSM ACTION FUNCTIONS --  BEGIN
func parseError(cfg *config, state int, symbol rune) error {
	return ErrParserError
}

func newSection(cfg *config, state int, symbol rune) error {
	if symbol != '[' {
		cfg.buffer += string(symbol)
	}

	cfg.meetSection = true
	return nil
}

func endSection(cfg *config, state int, symbol rune) error {
	var cur *tagSections

	sec := new(tagSections)

	if cfg.sections.next == nil { /* no section yet */
		cfg.sections.next = sec
	} else {
		cur = &cfg.sections
		for cur.next != nil {
			cur = cur.next
		}
		cur.next = sec
	}

	cfg.lastSection = sec
	cfg.lastSection.name = cfg.buffer
	cfg.buffer = ""

	return nil
}

func newOption(cfg *config, state int, symbol rune) error {
	cfg.keyName += string(symbol)

	return nil
}

func newValue(cfg *config, state int, symbol rune) error {
	if cfg.keyName != "" {
		option := new(tagOptions)
		if cfg.meetSection { /* already meet section(s) */
			if cfg.lastSection.options == nil {
				cfg.lastSection.options = option
			} else {
				cfg.lastOption.next = option
			}
		} else {
			cfg.sections.name = ""           //global section's name is empty
			if cfg.sections.options == nil { /* no option yet */
				cfg.sections.options = option
			} else {
				cur := cfg.sections.options
				for cur.next != nil {
					cur = cur.next
				}
				cur.next = option
			}
		}
		cfg.lastOption = option

		cfg.lastOption.name = cfg.keyName
		cfg.keyName = ""
	}
	if symbol != '=' {
		cfg.lastOption.value += string(symbol)
	}

	/* InStr state, if string contains multiple lines. */
	if symbol == '\n' {
		cfg.line++
	}

	return nil
}

func newLine(cfg *config, state int, symbol rune) error {
	cfg.line++
	return nil
}

//FSM ACTION FUNCTIONS --  END

func NewConfigFile(filename string) *Config {
	fin, errOpen := os.Open(filename)
	if errOpen != nil {
		Printf("NewConfigFile failed, err=%v\n", errOpen.Error())
		return nil
	}
	defer fin.Close()

	content, errRead := ioutil.ReadAll(fin)
	if errRead != nil {
		Printf("NewConfigFile failed, err=%v\n", errRead.Error())
		return nil
	}

    return NewConfigReader(bytes.NewReader(content))
}

func NewConfigReader(r io.Reader) *Config {
    return &Config{&config{reader:bufio.NewReader(r), line:1}}
}

func (self *Config) Parse() error {
	return self.config.parse()

}

func (self *config) parse() error {
	//Rules for describing the state change and associated actions for the FSM
	fsm := []struct {
		state    int  //current state
		c        rune //current rune read
		newState int  //next state
		//FSM action to execute for a certain state
		action func(cfg *config, state int, symbol rune) error
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
		{7, 0, 0, newLine},

		/* state = [Invalid] - 8 */
		{8, 0, -1, parseError},
	}

	state := 0

	reader := self.reader

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
					if actionErr := currFsm.action(self, state, ch); actionErr != nil {
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
func (self *Config) print() {
	self.config.cfgPrint()
}

/* for debug only */
func (self *config) cfgPrint() {
	var opt *tagOptions
	var sec *tagSections

	Println("\n==================RESULT:GLOBAL==================")

	if len(self.sections.name) == 0 { /* global option */
		opt = self.sections.options

		for opt != nil {
			Printf("options key[%v], value=[%v]\n", opt.name, opt.value)
			opt = opt.next
		} //end for
	}

	Printf("\n\n==================RESULT:SECTIONS==================")
	sec = self.sections.next
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

func (self *Config) Get(section, key string) (out string, err error) {
	return self.config.get(section, key)
}

func (self *config) get(section, key string) (out string, err error) {
	var opt *tagOptions
	var sec *tagSections

	out = ""

	if len(section) == 0 { /* global options */
		opt = self.sections.options

		for opt != nil {
			if opt.name == key {
				out = opt.value
				return
			}
			opt = opt.next
		} //end for

		err = ErrKeyNotFound
		return
	}

	sec = self.sections.next
	for sec != nil {
		if sec.name == section {
			opt = sec.options
			for opt != nil {
				if opt.name == key {
					out = opt.value
					return
				}

				opt = opt.next
			} //end inner for
		}

		sec = sec.next
	} //end for

    err = ErrKeyNotFound
	return
}

func main() {

	var out string
	var err error

	//read from string
	const testData = `
	server = "the forgotten server"
	
	[test_data]
	
	author = "raggaer"
	author_email = "xxx@gmail.com"
	author_age = 20
	`
	cfgReader := NewConfigReader(strings.NewReader(testData))

	if err = cfgReader.Parse(); err != nil {
		Printf("Error:%s", err.Error())
	}
	cfgReader.print()
	Println("\n\n\n\n")

	cfgFile := NewConfigFile("./test.ini")
	if err = cfgFile.Parse(); err != nil {
		Printf("Error:%s", err.Error())
		return
	}

	Println("\n==================GET RESULT:GLOBAL==================")
	var section string
	out, _ = cfgFile.Get("", "aa")
	Printf("Global section, aa=[%v]\n", out)

	out, _ = cfgFile.Get("", "a")
	Printf("Global section, a=[%v]\n", out)

	out, _ = cfgFile.Get("", "b")
	Printf("Global section, b=[%v]\n", out)

	out, _ = cfgFile.Get("", "c")
	Printf("Global section, c=[%v]\n", out)

	section = "ab;cdefg"
	Println("\n==================GET RESULT:[ab;cdefg]==================")
	out, _ = cfgFile.Get(section, "c")
	Printf("Named section[%v], c=[%v]\n", section, out)

	out, _ = cfgFile.Get(section, "d")
	Printf("Named section[%v], d=[%v]\n", section, out)

	out, _ = cfgFile.Get(section, "e")
	Printf("Named section[%v], e=[%v]\n", section, out)

	Println("\n==================GET RESULT:[xxxx]==================")
	section = "xxxx"
	out, _ = cfgFile.Get(section, "e")
	Printf("Named section[%v], e=[%v]\n", section, out)

	out, _ = cfgFile.Get(section, "m")
	Printf("Named section[%v], m=[%v]\n", section, out)

	out, _ = cfgFile.Get(section, "n")
	Printf("Named section[%v], n=[%v]\n", section, out)

	out, err  = cfgFile.Get(section, "ffff")
    if err == ErrKeyNotFound {
        Printf("Named section[%v], ffff is not found\n", section)
    } else {
        Printf("Named section[%v], ffff=[%v]\n", section, out)
    }

	Println("\n==================[DEBUG cfgFile]==================\n")
	cfgFile.print()
}
