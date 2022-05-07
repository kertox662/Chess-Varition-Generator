package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"go.melnyk.org/mlog"
	"go.melnyk.org/mlog/console"
)

type Engine struct {
	path                       string
	proc                       *exec.Cmd
	threads, memory, pv        int
	isRunning                  bool
	pipeReader                 *bufio.Reader
	pipeWriter                 io.Writer
	printAll, printVarProgress bool
	logger                     mlog.Logger
}

var lb = console.NewLogbook(os.Stderr)

func init() {
	lb.SetLevel(mlog.Default, mlog.Info)
}

func NewEngine(path string) *Engine {
	return &Engine{
		path:    path,
		threads: 8,
		memory:  2048,
		pv:      5,
		logger:  lb.Joiner().Join("engine"),
	}
}

func (eng *Engine) Start() error {
	eng.proc = exec.Command(eng.path)
	readPipe, err := eng.proc.StdoutPipe()
	if err != nil {
		return err
	}
	writePipe, err := eng.proc.StdinPipe()
	if err != nil {
		return err
	}

	if err := eng.proc.Start(); err != nil {
		return err
	}

	eng.pipeReader = bufio.NewReader(readPipe)
	eng.pipeWriter = writePipe
	eng.isRunning = true

	eng.write("uci")
	eng.SetOption("Threads", strconv.Itoa(eng.threads))
	eng.SetOption("Hash", strconv.Itoa(eng.memory))
	eng.SetOption("MultiPV", strconv.Itoa(eng.pv))
	eng.write("isready")
	eng.write("ucinewgame")
	return eng.SkipUntilSubstring("readyok")
}

func (eng *Engine) write(msg string) error {
	n, err := eng.pipeWriter.Write([]byte(msg + "\n"))
	if err != nil {
		return fmt.Errorf("error writing to engine: %w", err)
	}
	if n != len([]byte(msg)) {
		return fmt.Errorf("error writing to engine: number of bytes written not length of message")
	}

	return nil
}

func (eng *Engine) read() (string, error) {
	line, err := eng.pipeReader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("error writing to engine: %w", err)
	}
	if eng.printAll {
		eng.logger.Info(line)
	}

	return line, nil
}

func (eng *Engine) SkipUntilSubstring(substr string) error {
	return eng.ReadUntilSubstring(substr, func(s string) {})
}

func (eng *Engine) ReadUntilSubstring(substr string, fn func(string)) error {
	line, err := eng.read()
	if err != nil {
		return err
	}

	for !strings.Contains(line, substr) {
		fn(line)
		line, err = eng.read()
		if err != nil {
			return err
		}
	}

	return nil
}

func (eng *Engine) SetOption(name, value string) {
	eng.write(fmt.Sprintf("setoption name %s value %s", name, value))
}

func (eng *Engine) SetThreads(threads int) *Engine {
	if eng.isRunning {
		eng.SetOption("Threads", strconv.Itoa(threads))
	}
	eng.threads = threads
	return eng
}

func (eng *Engine) SetMemory(mem int) *Engine {
	if eng.isRunning {
		eng.SetOption("Hash", strconv.Itoa(mem))
	}
	eng.memory = mem
	return eng
}

func (eng *Engine) SetPV(pv int) *Engine {
	if eng.isRunning {
		eng.SetOption("MultiPV", strconv.Itoa(pv))
	}
	eng.pv = pv
	return eng
}

func (eng *Engine) SetPrintAll(pa bool) *Engine {
	eng.printAll = pa
	return eng
}

func (eng *Engine) SetPrintVarProgresss(pvp bool) *Engine {
	eng.printVarProgress = pvp
	return eng
}

func (eng *Engine) SetMoves(mvs Moves) {
	eng.write(fmt.Sprintf("position startpos moves %s", mvs.String()))
}

func (eng *Engine) SetFen(fen string) {
	eng.write(fmt.Sprintf("position fen %s", fen))
}

var fenRE = regexp.MustCompile(`Fen: (.+) \d+ \d+`)

func (eng *Engine) GetFen() string {
	eng.write("d")
	var fen string
	eng.ReadUntilSubstring("Checkers:", func(s string) {
		if m := fenRE.FindStringSubmatch(s); m != nil {
			fen = m[1]
		}
	})

	return fen
}

func (eng *Engine) StartSearch(depth int) {
	eng.write(fmt.Sprintf("go depth %d", depth))
}

func (eng *Engine) PrintPos(mvs Moves) {
	eng.SetMoves(mvs)
	eng.write("d")
	eng.ReadUntilSubstring("Checkers:", func(s string) { fmt.Println(s) })
}

func (eng *Engine) FindBestMove(initial Moves, depth, numLines int) (Moves, error) {
	eng.SetOption("MultiPV", strconv.Itoa(numLines))
	eng.SetMoves(initial)
	eng.StartSearch(depth)
	res := make(map[string]int)
	err := eng.ReadUntilSubstring("bestmove", func(s string) {
		if mv := parseFirstMove(s, depth); mv != nil {
			res[mv.move] = mv.eval
		}
	})
	if err != nil {
		return nil, fmt.Errorf("error finding best moves: %w", err)
	}

	mvs := make(Moves, len(res))
	i := 0
	for k, v := range res {
		mvs[i] = Move{move: k, eval: v}
		i++
	}

	return mvs, nil
}

var re *regexp.Regexp = regexp.MustCompile(`.* depth (\d+) .* cp (-?\d+) .* pv (.+)`)

func parseFirstMove(line string, depth int) *Move {
	match := re.FindStringSubmatch(line)
	if match == nil {
		return nil
	}
	dp, err := strconv.Atoi(match[1])
	if err != nil || dp != depth {
		return nil
	}

	cp, err := strconv.Atoi(match[2])
	if err != nil {
		return nil
	}

	spaceInd := strings.Index(match[3], " ")
	if spaceInd == -1 {
		return &Move{
			move: match[3],
			eval: cp,
		}
	}

	return &Move{
		move: match[3][:spaceInd],
		eval: cp,
	}
}

func (eng *Engine) MakeVariations(inital Moves, varDepth, engineDepth int, isWhite bool) (map[string]bool, error) {
	onlyBest := len(inital)%2 == 1
	if isWhite {
		onlyBest = !onlyBest
	}
	res := make(map[string]bool)
	cache := make(map[string]bool)
	err := eng.makeVariations(inital, varDepth, engineDepth, res, cache, onlyBest)
	if err != nil {
		return res, fmt.Errorf("error making variations: %w", err)
	}
	return res, nil
}

const cpThreshold = -250

func (eng *Engine) makeVariations(inital Moves, varDepth, engineDepth int, results, posSet map[string]bool, onlyBest bool) error {
	if varDepth == 0 {
		results[inital.String()] = true
		return nil
	}
	numLines := eng.pv
	if onlyBest {
		numLines = 1
	}

	eng.SetMoves(inital)
	fen := eng.GetFen()
	if posSet[fen] {
		results[inital.String()] = true
		return nil
	}
	posSet[fen] = true

	bestMoves, err := eng.FindBestMove(inital, engineDepth, numLines)
	if err != nil {
		return err
	}

	if eng.printVarProgress {
		eng.logger.Info("Best moves calculated:")
		eng.logger.Verbose("INITIAL:" + inital.String())
		eng.logger.Info("BEST:" + bestMoves.String())
		eng.logger.Info("Depth Left to go:" + strconv.Itoa(varDepth-1))
		if len(results)%100 == 0 {
			eng.logger.Info(fmt.Sprintf("Variations Calculated: %d", len(results)))
		}
	}

	if len(bestMoves) == 0 {
		results[inital.String()] = true
	} else if onlyBest {
		var mv Move
		for _, m := range bestMoves {
			mv = m
		}
		if mv.eval < cpThreshold {
			return nil
		}
		return eng.makeVariations(append(inital, mv), varDepth-1, engineDepth, results, posSet, !onlyBest)
	} else {
		goodEval := false
		for _, mv := range bestMoves {
			if mv.eval > cpThreshold {
				goodEval = true
				if err := eng.makeVariations(append(inital, mv), varDepth-1, engineDepth, results, posSet, !onlyBest); err != nil {
					return err
				}
			} else {
				if !onlyBest {
					if err := eng.makeVariations(append(inital, mv), 1, engineDepth, results, posSet, true); err != nil {
						return err
					}
				}
			}
		}
		if !goodEval {
			results[inital.String()] = true
		}
	}

	return nil
}
