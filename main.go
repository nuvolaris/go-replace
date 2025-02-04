package replace

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	flags "github.com/jessevdk/go-flags"
	"github.com/remeh/sizedwaitgroup"
)

const (
	Author = "webdevops.io"
)

var (
	// Git version information
	gitCommit = "<unknown>"
	gitTag    = "<unknown>"
)

type Changeset struct {
	SearchPlain string
	Search      *regexp.Regexp
	Replace     string
	MatchFound  bool
}

type Changeresult struct {
	File   Fileitem
	Output string
	Status bool
	Error  error
}

type Fileitem struct {
	Path   string
	Output string
}

var opts struct {
	ThreadCount        int    `           long:"threads"                       description:"Set thread concurrency for replacing in multiple files at same time" default:"20"`
	Mode               string `short:"m"  long:"mode"                          description:"replacement mode - replace: replace match with term; line: replace line with term; lineinfile: replace line with term or if not found append to term to file; template: parse content as golang template, search value have to start uppercase" default:"replace" choice:"replace" choice:"line" choice:"lineinfile" choice:"template"`
	ModeIsReplaceMatch bool
	ModeIsReplaceLine  bool
	ModeIsLineInFile   bool
	ModeIsTemplate     bool
	Search             []string `short:"s"  long:"search"                        description:"search term"`
	Replace            []string `short:"r"  long:"replace"                       description:"replacement term"`
	LineinfileBefore   string   `           long:"lineinfile-before"             description:"add line before this regex"`
	LineinfileAfter    string   `           long:"lineinfile-after"              description:"add line after this regex"`
	CaseInsensitive    bool     `short:"i"  long:"case-insensitive"              description:"ignore case of pattern to match upper and lowercase characters"`
	Stdin              bool     `           long:"stdin"                         description:"process stdin as input"`
	Output             string   `short:"o"  long:"output"                        description:"write changes to this file (in one file mode)"`
	OutputStripFileExt string   `           long:"output-strip-ext"              description:"strip file extension from written files (also available in multi file mode)"`
	Once               string   `           long:"once"                          description:"replace search term only one in a file, keep duplicaes (keep, default) or remove them (unique)" optional:"true" optional-value:"keep" choice:"keep" choice:"unique"`
	Regex              bool     `           long:"regex"                         description:"treat pattern as regex"`
	RegexBackref       bool     `           long:"regex-backrefs"                description:"enable backreferences in replace term"`
	RegexPosix         bool     `           long:"regex-posix"                   description:"parse regex term as POSIX regex"`
	Path               string   `           long:"path"                          description:"use files in this path"`
	PathPattern        string   `           long:"path-pattern"                  description:"file pattern (* for wildcard, only basename of file)"`
	PathRegex          string   `           long:"path-regex"                    description:"file pattern (regex, full path)"`
	IgnoreEmpty        bool     `           long:"ignore-empty"                  description:"ignore empty file list, otherwise this will result in an error"`
	Verbose            bool     `short:"v"  long:"verbose"                       description:"verbose mode"`
	DryRun             bool     `           long:"dry-run"                       description:"dry run mode"`
	ShowVersion        bool     `short:"V"  long:"version"                       description:"show version and exit"`
	ShowOnlyVersion    bool     `           long:"dumpversion"                   description:"show only version number and exit"`
	ShowHelp           bool     `short:"h"  long:"help"                          description:"show this help message"`
}

var pathFilterDirectories = []string{"autom4te.cache", "blib", "_build", ".bzr", ".cdv", "cover_db", "CVS", "_darcs", "~.dep", "~.dot", ".git", ".hg", "~.nib", ".pc", "~.plst", "RCS", "SCCS", "_sgbak", ".svn", "_obj", ".idea"}

// Apply changesets to file
func applyChangesetsToFile(fileitem Fileitem, changesets []Changeset) (string, bool, error) {
	var (
		output string = ""
		status bool   = true
	)

	// try open file
	file, err := os.Open(fileitem.Path)
	if err != nil {
		return output, false, err
	}

	writeBufferToFile := false
	var buffer bytes.Buffer

	r := bufio.NewReader(file)
	line, e := Readln(r)
	for e == nil {
		newLine, lineChanged, skipLine := applyChangesetsToLine(line, changesets)

		if lineChanged || skipLine {
			writeBufferToFile = true
		}

		if !skipLine {
			buffer.WriteString(newLine + "\n")
		}

		line, e = Readln(r)
	}
	file.Close()

	// --mode=lineinfile
	if opts.ModeIsLineInFile {
		lifBuffer, lifStatus := HandleLineInFile(changesets, buffer)
		if lifStatus {
			buffer.Reset()
			buffer.WriteString(lifBuffer.String())
			writeBufferToFile = lifStatus
		}
	}

	// --output
	// --output-strip-ext
	// enforcing writing of file (creating new file)
	if opts.Output != "" || opts.OutputStripFileExt != "" {
		writeBufferToFile = true
	}

	if writeBufferToFile {
		output, status = WriteContentToFile(fileitem, buffer)
	} else {
		output = fmt.Sprintf("%s no match", fileitem.Path)
	}

	return output, status, err
}

// Apply changesets to file
func applyTemplateToFile(fileitem Fileitem, changesets []Changeset) (string, bool, error) {
	// try open file
	buffer, err := os.ReadFile(fileitem.Path)
	if err != nil {
		return "", false, err
	}

	content, err := ParseContentAsTemplate(string(buffer), changesets)
	if err != nil {
		return "", false, err
	}

	output, status := WriteContentToFile(fileitem, content)

	return output, status, err
}

func applyChangesetsToLine(line string, changesets []Changeset) (string, bool, bool) {
	changed := false
	skipLine := false

	for i, changeset := range changesets {
		// --once, only do changeset once if already applied to file
		if opts.Once != "" && changeset.MatchFound {
			// --once=unique, skip matching lines
			if opts.Once == "unique" && SearchMatch(line, changeset) {
				// matching line, not writing to buffer as requsted
				skipLine = true
				changed = true
				break
			}
		} else {
			// search and replace
			if SearchMatch(line, changeset) {
				// --mode=line or --mode=lineinfile
				if opts.ModeIsReplaceLine || opts.ModeIsLineInFile {
					if opts.RegexBackref {
						// get match
						line = string(changeset.Search.Find([]byte(line)))

						// replace regex backrefs in match
						line = changeset.Search.ReplaceAllString(line, changeset.Replace)
					} else {
						// replace whole line with replace term
						line = changeset.Replace
					}
				} else {
					// replace only term inside line
					line = ReplaceText(line, changeset)
				}

				changesets[i].MatchFound = true
				changed = true
			}
		}
	}

	return line, changed, skipLine
}

// Build search term
// Compiles regexp if regexp is used
func buildSearchTerm(term string) *regexp.Regexp {
	var ret *regexp.Regexp
	var regex string

	// --regex
	if opts.Regex {
		// use search term as regex
		regex = term
	} else {
		// use search term as normal string, escape it for regex usage
		regex = regexp.QuoteMeta(term)
	}

	// --ignore-case
	if opts.CaseInsensitive {
		regex = "(?i:" + regex + ")"
	}

	// --verbose
	if opts.Verbose {
		logMessage(fmt.Sprintf("Using regular expression: %s", regex))
	}

	// --regex-posix
	if opts.RegexPosix {
		ret = regexp.MustCompilePOSIX(regex)
	} else {
		ret = regexp.MustCompile(regex)
	}

	return ret
}

// handle special cli options
// eg. --help
//
//	--version
//	--path
//	--mode=...
func handleSpecialCliOptions(args []string) (bool, error) {
	// --dumpversion
	if opts.ShowOnlyVersion {
		fmt.Println(gitTag)
		return true, nil
	}

	// --version
	if opts.ShowVersion {
		fmt.Printf("go-replace version %s (%s)\n", gitTag, gitCommit)
		fmt.Printf("Copyright (C) 2022 %s\n", Author)
		return true, nil
	}

	// --help
	if opts.ShowHelp {
		argparser.WriteHelp(os.Stdout)
		return true, nil
	}

	// --mode
	switch mode := opts.Mode; mode {
	case "replace":
		opts.ModeIsReplaceMatch = true
		opts.ModeIsReplaceLine = false
		opts.ModeIsLineInFile = false
		opts.ModeIsTemplate = false
	case "line":
		opts.ModeIsReplaceMatch = false
		opts.ModeIsReplaceLine = true
		opts.ModeIsLineInFile = false
		opts.ModeIsTemplate = false
	case "lineinfile":
		opts.ModeIsReplaceMatch = false
		opts.ModeIsReplaceLine = false
		opts.ModeIsLineInFile = true
		opts.ModeIsTemplate = false
	case "template":
		opts.ModeIsReplaceMatch = false
		opts.ModeIsReplaceLine = false
		opts.ModeIsLineInFile = false
		opts.ModeIsTemplate = true
	}

	// --output
	if opts.Output != "" && len(args) > 1 {
		return true, errors.New("only one file is allowed when using --output")
	}

	if opts.LineinfileBefore != "" || opts.LineinfileAfter != "" {
		if !opts.ModeIsLineInFile {
			return true, errors.New("--lineinfile-after and --lineinfile-before only valid in --mode=lineinfile")
		}

		if opts.LineinfileBefore != "" && opts.LineinfileAfter != "" {
			return true, errors.New("only --lineinfile-after or --lineinfile-before is allowed in --mode=lineinfile")
		}
	}

	return false, nil
}

func actionProcessStdinReplace(changesets []Changeset) int {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()

		newLine, _, skipLine := applyChangesetsToLine(line, changesets)

		if !skipLine {
			fmt.Println(newLine)
		}
	}

	return 0
}

func actionProcessStdinTemplate(changesets []Changeset) (int, error) {
	var buffer bytes.Buffer

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		buffer.WriteString(scanner.Text() + "\n")
	}

	content, err := ParseContentAsTemplate(buffer.String(), changesets)
	if err != nil {
		return 1, err
	}

	fmt.Print(content.String())

	return 0, nil
}

func actionProcessFiles(changesets []Changeset, fileitems []Fileitem) (int, error) {
	// check if there is at least one file to process
	if len(fileitems) == 0 {
		if opts.IgnoreEmpty {
			// no files found, but we should ignore empty filelist
			logMessage("No files found, requsted to ignore this")
			return 0, nil
		} else {
			// no files found, print error and exit with error code
			return 1, errors.New("No files specified")
		}
	}

	swg := sizedwaitgroup.New(8)
	results := make(chan Changeresult, len(fileitems))

	// process file list
	for _, file := range fileitems {
		swg.Add()
		go func(file Fileitem, changesets []Changeset) {
			var (
				err    error
				output string
				status bool
			)

			if opts.ModeIsTemplate {
				output, status, err = applyTemplateToFile(file, changesets)
			} else {
				output, status, err = applyChangesetsToFile(file, changesets)
			}

			results <- Changeresult{file, output, status, err}
			swg.Done()
		}(file, changesets)
	}

	// wait for all changes to be processed
	swg.Wait()
	close(results)

	// show results
	errorCount := 0
	for result := range results {
		if result.Error != nil {
			logError(result.Error)
			errorCount++
		} else if opts.Verbose {
			title := fmt.Sprintf("%s:", result.File.Path)

			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, title)
			fmt.Fprintln(os.Stderr, strings.Repeat("-", len(title)))
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, result.Output)
			fmt.Fprintln(os.Stderr, "")
		}
	}

	if errorCount >= 1 {
		fmt.Fprintf(os.Stderr, "[ERROR] %s failed with %d error(s)\n", argparser.Command.Name, errorCount)
		return 1, errors.New("One or more files failed")
	}

	return 0, nil
}

func buildChangesets() ([]Changeset, error) {
	var changesets []Changeset

	if !opts.ModeIsTemplate {
		if len(opts.Search) == 0 || len(opts.Replace) == 0 {
			// error: unequal numbers of search and replace options
			return nil, errors.New("Missing either --search or --replace for this mode")
		}
	}

	// check if search and replace options have equal lenght (equal number of options)
	if len(opts.Search) != len(opts.Replace) {
		// error: unequal numbers of search and replace options
		return nil, errors.New("Unequal numbers of search or replace options")
	}

	// build changesets
	for i := range opts.Search {
		search := opts.Search[i]
		replace := opts.Replace[i]

		changeset := Changeset{search, buildSearchTerm(search), replace, false}
		changesets = append(changesets, changeset)
	}

	return changesets, nil
}

func buildFileitems(args []string) []Fileitem {
	var (
		fileitems []Fileitem
		file      Fileitem
	)

	// Build filelist from arguments
	for _, filepath := range args {
		file = Fileitem{filepath, filepath}

		if opts.Output != "" {
			// use specific output
			file.Output = opts.Output
		} else if opts.OutputStripFileExt != "" {
			// remove file ext from saving destination
			file.Output = strings.TrimSuffix(file.Output, opts.OutputStripFileExt)
		} else if strings.Contains(filepath, ":") {
			// argument like "source:destination"
			split := strings.SplitN(filepath, ":", 2)

			file.Path = split[0]
			file.Output = split[1]
		}

		fileitems = append(fileitems, file)
	}

	// --path parsing
	if opts.Path != "" {
		SearchFilesInPath(opts.Path, func(f os.FileInfo, filepath string) {
			file := Fileitem{filepath, filepath}

			if opts.OutputStripFileExt != "" {
				// remove file ext from saving destination
				file.Output = strings.TrimSuffix(file.Output, opts.OutputStripFileExt)
			}

			// no colon parsing here

			fileitems = append(fileitems, file)
		})
	}

	return fileitems
}

var argparser *flags.Parser

func ReplaceMain() (int, error) {
	argparser = flags.NewParser(&opts, flags.PassDoubleDash)
	args, err := argparser.Parse()

	shouldExit, err := handleSpecialCliOptions(args)
	if err != nil {
		return 1, err
	} else if shouldExit {
		return 0, nil
	}

	// check if there is an parse error
	if err != nil {
		return 1, err
	}

	changesets, err := buildChangesets()
	if err != nil {
		return 1, err
	}
	fileitems := buildFileitems(args)

	exitMode := 0
	if opts.Stdin {
		if opts.ModeIsTemplate {
			// use stdin as input
			exitMode, err = actionProcessStdinTemplate(changesets)
			if err != nil {
				return exitMode, err
			}
		} else {
			// use stdin as input
			exitMode = actionProcessStdinReplace(changesets)
		}
	} else {
		// use and process files (see args)
		exitMode, err = actionProcessFiles(changesets, fileitems)
		if err != nil {
			return exitMode, err
		}
	}

	return exitMode, nil
}
