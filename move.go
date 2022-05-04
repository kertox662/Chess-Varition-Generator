package main

import "strings"

type Move struct {
	move string
	eval int
}

type Moves []Move

func MovesFromString(str string) Moves {
	s := strings.Split(str, " ")
	mvs := make(Moves, len(s))
	for i, mv := range s {
		mvs[i] = Move{
			move: mv,
			eval: 0,
		}
	}
	return mvs
}

func (mvs Moves) String() string {
	s := make([]string, len(mvs))
	for i := range mvs {
		s[i] = mvs[i].move
	}
	return strings.Join(s, " ")
}
