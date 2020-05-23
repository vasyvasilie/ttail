package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"time"
)

type logParser struct {
	r     *regexp.Regexp
	l     string
	after time.Time
}

type logParsers map[string]logParser

func listParsers(parsers logParsers) {
	fmt.Println("available formats:")
	for i := range parsers {
		fmt.Println("name:", i, "format:", parsers[i].l)
	}

}

func createParsers(secs uint) logParsers {
	// parsers here
	ps := logParsers{
		"nginx": logParser{
			r: regexp.MustCompile(`\d{2}\/[a-zA-Z]{3}\/\d{4}:\d{2}:\d{2}:\d{2}\ \+\d{4}`),
			l: "02/Jan/2006:15:04:05 -0700",
		},
	}

	// add after to all parsers
	now := time.Now()
	checkAfter := now.Add(time.Duration(-secs) * time.Second)
	for name := range ps {
		p := ps[name]
		p.after = checkAfter
		ps[name] = p
	}
	return ps
}

func (p logParser) maybeAppend(res []string, str string) ([]string, bool) {
	if p.r.MatchString(str) {
		timeInStr, err := time.Parse(p.l, p.r.FindString(str))
		// not parseable time?
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if timeInStr.After(p.after) {
			return append(res, str), true
		}
	}
	return res, false
}

func main() {
	secs := flag.Uint("n", 300, "seconds")
	lfile := flag.String("f", "access_.log", "path to file")
	ltype := flag.String("t", "nginx", "log format")
	bufSize := flag.Int("b", 4096, "buffer for read (bytes)")
	memSize := flag.Int("m", 4096, "max memory (bytes)")
	flag.Parse()

	parsers := createParsers(*secs)
	parser, ok := parsers[*ltype]
	if !ok {
		listParsers(parsers)
		os.Exit(1)
	}

	f, err := os.Open(*lfile)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer f.Close()

	s, err := f.Stat()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	offset := int(s.Size())
	nl := []byte("\n")
	buf := make([]byte, *bufSize)

	var res []string
	var found bool
	nextBuf := make([]byte, 0)
	for {
		_, err := f.ReadAt(buf, int64(offset))

		if err != nil {
			if err == io.EOF {
				offset -= *bufSize
				if offset < 0 {
					buf = make([]byte, offset+*bufSize)
					offset = 0
				}
				continue
			}
			fmt.Println(err)
			os.Exit(1)
		}

		combinedBuf := append(buf, nextBuf...)
		firstIndex := bytes.Index(combinedBuf, nl)
		if firstIndex == -1 {
			toReserve := len(nextBuf) + len(buf)
			if toReserve > *memSize {
				fmt.Println("max memory exceeded: limit:", *memSize, ", wanted:", toReserve)
				os.Exit(1)
			}
			nextBuf = make([]byte, toReserve)
			copy(nextBuf, combinedBuf)
			if offset == 0 {
				if len(nextBuf) > 0 {
					res, _ = parser.maybeAppend(res, string(nextBuf))
				}
				break
			}
			offset -= *bufSize
			if offset < 0 {
				buf = make([]byte, offset+*bufSize)
				offset = 0
			}
			continue
		}

		currBuf := combinedBuf[firstIndex:]
		if len(combinedBuf[:firstIndex]) > 0 {
			nextBuf = make([]byte, len(combinedBuf[:firstIndex]))
			copy(nextBuf, combinedBuf[:firstIndex])
		} else {
			nextBuf = make([]byte, 0)
		}

		splitted := bytes.Split(currBuf, nl)
		for i := len(splitted) - 1; i >= 0; i-- {
			if len(splitted[i]) == 0 {
				continue
			}
			res, found = parser.maybeAppend(res, string(splitted[i]))
		}
		if offset == 0 {
			if len(nextBuf) > 0 {
				res, found = parser.maybeAppend(res, string(nextBuf))
			}
			break
		}

		// this code is not good cause of breaking execution when small buffer
		// is set (less than max lenght of string in log), but this helps not
		// to read whole file instead of buffer with last found line
		// i hope i will not use it with small buffer
		if !found {
			break
		}

		buf = make([]byte, *bufSize)
		offset -= *bufSize
		if offset < 0 {
			buf = make([]byte, offset+*bufSize)
			offset = 0
		}
	}

	// reverse array
	for i, j := 0, len(res)-1; i < j; i, j = i+1, j-1 {
		res[i], res[j] = res[j], res[i]
	}

	for i := range res {
		fmt.Println(res[i])
	}
}
