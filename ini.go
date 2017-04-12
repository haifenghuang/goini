package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"
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

//Config struct represent a Configration
type Config struct {
	*config
}

//This is the real representation of a config
type config struct {
	reader      *bufio.Reader
	sections    tagSections
	lastSection *tagSections
	lastOption  *tagOptions
	keyName     string
	buffer      string
	meetSection bool
	line        int
	parsed      bool //prevent multiple call to Parse
}

var (
	//ErrParserError is the error returned when parsing error occurred
	ErrParserError = errors.New("Parse error")
	//ErrMalFormed is the error returned if configuration file is malformed
	ErrMalFormed = errors.New("Input is malformed, ends inside comment or literal")
	//ErrKeyNotFound is the error returned if searched key not found in configuration
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

//NewConfigFile constrcut a new 'Config' object from 'filename'
func NewConfigFile(filename string) *Config {
	fin, errOpen := os.Open(filename)
	if errOpen != nil {
		fmt.Printf("NewConfigFile failed, err=%v\n", errOpen.Error())
		return nil
	}
	defer fin.Close()

	content, errRead := ioutil.ReadAll(fin)
	if errRead != nil {
		fmt.Printf("NewConfigFile failed, err=%v\n", errRead.Error())
		return nil
	}

	return NewConfigReader(bytes.NewReader(content))
}

//NewConfigReader constrcut a new config file object from 'r'(io.Reader)
func NewConfigReader(r io.Reader) *Config {
	return &Config{&config{reader: bufio.NewReader(r), line: 1}}
}

//Parse parse a configuration file
func (cfg *Config) Parse() error {
	return cfg.config.parse()

}

func (cfg *config) parse() error {
	if cfg.parsed {
		return nil
	}
	cfg.parsed = true

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

	reader := cfg.reader

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
					if actionErr := currFsm.action(cfg, state, ch); actionErr != nil {
						return actionErr
					}
				}
				state = currFsm.newState
				break
			} //end if
		} //inner for
	} //outer for

	if state != 0 {
		fmt.Printf("state is %d\n", state)
		return ErrMalFormed
	}

	return nil
}

//Bool returns an boolean value for a 'key' in 'section', if failed, will return 'def'
func (cfg *Config) Bool(section, key string, def bool) (out bool) {
	out = def
	outStr, err := cfg.Get(section, key)
	if err != nil {
		return
	}

	out, err = strconv.ParseBool(outStr)

	return
}

//Int returns an int value for a 'key' in 'section', if failed, will return 'def'
func (cfg *Config) Int(section, key string, def int) (out int) {
	out = def
	outStr, err := cfg.Get(section, key)
	if err != nil {
		return
	}

	out64, err := strconv.ParseInt(outStr, 0, 64)
	if err != nil {
		return
	}
	out = int(out64)

	return
}

//Int64 returns an int value for a 'key' in 'section', if failed, will return 'def'
func (cfg *Config) Int64(section, key string, def int64) (out int64) {
	out = def
	outStr, err := cfg.Get(section, key)
	if err != nil {
		return
	}

	out64, err := strconv.ParseInt(outStr, 0, 64)
	if err != nil {
		return
	}
	out = out64

	return
}

//Uint returns an uint value for a 'key' in 'section', if failed, will return 'def'
func (cfg *Config) Uint(section, key string, def uint) (out uint) {
	out = def
	outStr, err := cfg.Get(section, key)
	if err != nil {
		return
	}

	out64, err := strconv.ParseUint(outStr, 0, 64)
	if err != nil {
		return
	}
	out = uint(out64)

	return
}

//Uint64 returns an uint64 value for a 'key' in 'section', if failed, will return 'def'
func (cfg *Config) Uint64(section, key string, def uint64) (out uint64) {
	out = def
	outStr, err := cfg.Get(section, key)
	if err != nil {
		return
	}

	out64, err := strconv.ParseUint(outStr, 0, 64)
	if err != nil {
		return
	}
	out = uint64(out64)

	return
}

//Float64 returns an float64 value for a 'key' in 'section', if failed, will return 'def'
func (cfg *Config) Float64(section, key string, def float64) (out float64) {
	out = def
	outStr, err := cfg.Get(section, key)
	if err != nil {
		return
	}

	out64, err := strconv.ParseFloat(outStr, 64)
	if err != nil {
		return
	}
	out = out64

	return
}

//Duration returns an time.Duration value for a 'key' in 'section', if failed, will return 'def'
func (cfg *Config) Duration(section, key string, def time.Duration) (out time.Duration) {
	out = def
	outStr, err := cfg.Get(section, key)
	if err != nil {
		return
	}

	outDuration, err := time.ParseDuration(outStr)
	if err != nil {
		return
	}
	out = outDuration

	return
}

//Array returns an array for a 'key' in 'section', if failed, will return [].
//e.g. key1=1,2,3,4 returns ["1", "2", "3", "4"]
//Note: it assumes the values are all "string"s
func (cfg *Config) Array(section, key string) []string {
	value, err := cfg.Get(section, key)
	if err != nil {
		return make([]string, 0)
	}

	return strings.Split(value, ",")

}

//Map returns a map for a 'key' in 'section', if failed, will return an empty map[string]string.
//e.g. demo = key1:value1, key2, value2, key3, value3
func (cfg *Config) Map(section, key string) map[string]string {
	outStr, err := cfg.Get(section, key)
	if err != nil {
		return make(map[string]string, 0)
	}

	subStrs := strings.Split(outStr, ",")
	out := make(map[string]string, len(subStrs))

	for _, subStr := range subStrs {
		pair := strings.Split(subStr, ":")
		out[pair[0]] = pair[1]
	}
	return out
}

//Get search a 'key' from 'section', returns key value 'out' and 'err'
func (cfg *Config) Get(section, key string) (out string, err error) {
	return cfg.config.get(section, key)
}

func (cfg *config) get(section, key string) (out string, err error) {
	var opt *tagOptions
	var sec *tagSections

	out = ""

	if len(section) == 0 { /* global options */
		opt = cfg.sections.options

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

	sec = cfg.sections.next
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

/* for debug only */
func (cfg *Config) print() {
	cfg.config.cfgPrint()
}

/* for debug only */
func (cfg *config) cfgPrint() {
	var opt *tagOptions
	var sec *tagSections

	fmt.Println("\n==================RESULT:GLOBAL==================")

	if len(cfg.sections.name) == 0 { /* global option */
		opt = cfg.sections.options

		for opt != nil {
			fmt.Printf("options key[%v], value=[%v]\n", opt.name, opt.value)
			opt = opt.next
		} //end for
	}

	fmt.Printf("\n\n==================RESULT:SECTIONS==================")
	sec = cfg.sections.next
	for sec != nil {
		fmt.Printf("\nsection name=[%v]\n", sec.name)

		opt = sec.options
		for opt != nil {
			fmt.Printf("\toptions key[%v], value=[%v]\n", opt.name, opt.value)
			opt = opt.next
		} //end inner for

		sec = sec.next
	} //end for
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
		fmt.Printf("Error:%s", err.Error())
	}
	cfgReader.print()
	fmt.Printf("\n\n\n\n")

	cfgFile := NewConfigFile("./test.ini")
	if err = cfgFile.Parse(); err != nil {
		fmt.Printf("Error:%s", err.Error())
		return
	}

	fmt.Println("\n==================GET RESULT:GLOBAL==================")
	var section string
	out, _ = cfgFile.Get("", "aa")
	fmt.Printf("Global section, aa=[%v]\n", out)

	out, _ = cfgFile.Get("", "a")
	fmt.Printf("Global section, a=[%v]\n", out)

	out, _ = cfgFile.Get("", "b")
	fmt.Printf("Global section, b=[%v]\n", out)

	out, _ = cfgFile.Get("", "c")
	fmt.Printf("Global section, c=[%v]\n", out)

	section = "ab;cdefg"
	fmt.Println("\n==================GET RESULT:[ab;cdefg]==================")
	out, _ = cfgFile.Get(section, "c")
	fmt.Printf("Named section[%v], c=[%v]\n", section, out)

	out, _ = cfgFile.Get(section, "d")
	fmt.Printf("Named section[%v], d=[%v]\n", section, out)

	out, _ = cfgFile.Get(section, "e")
	fmt.Printf("Named section[%v], e=[%v]\n", section, out)

	fmt.Println("\n==================GET RESULT:[xxxx]==================")
	section = "xxxx"
	out, _ = cfgFile.Get(section, "e")
	fmt.Printf("Named section[%v], e=[%v]\n", section, out)

	out, _ = cfgFile.Get(section, "m")
	fmt.Printf("Named section[%v], m=[%v]\n", section, out)

	outUint := cfgFile.Int(section, "m", 10)
	fmt.Printf("outUint=[%v]\n", outUint)

	lists := cfgFile.Array(section, "list")
	for idx, list := range lists {
		fmt.Printf("list[%d] = [%v]\n", idx, list)
	}

	maps := cfgFile.Map(section, "map")
	for key, value := range maps {
		fmt.Printf("[%v] = [%v]\n", key, value)
	}

	out, _ = cfgFile.Get(section, "n")
	fmt.Printf("Named section[%v], n=[%v]\n", section, out)

	out, err = cfgFile.Get(section, "ffff")
	if err == ErrKeyNotFound {
		fmt.Printf("Named section[%v], ffff is not found\n", section)
	} else {
		fmt.Printf("Named section[%v], ffff=[%v]\n", section, out)
	}

	fmt.Printf("\n==================[DEBUG cfgFile]==================\n")
	cfgFile.print()
}
