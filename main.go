package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

type Config struct {
	EngineSettings struct {
		Memory        *int `json:"memory"`
		Threads       *int `json:"threads"`
		PrintAll      bool `json:"print-all"`
		PrintProgress bool `json:"print-progress"`
	} `json:"engine-settings"`
	VariationConfig struct {
		InitialMoves   string `json:"initial-moves"`
		EngineDepth    int    `json:"engine-depth"`
		VariationDepth int    `json:"variation-depth"`
		IsWhite        bool   `json:"is-white"`
	} `json:"variation-config"`
	EnginePath string `json:"engine-path"`
	OutputPath string `json:"output-path"`
}

func main() {
	file, err := os.Open("config.json")
	defer file.Close()
	if err != nil {
		log.Fatal("error opening config.json file:", err)
	}
	cfg := &Config{}
	err = json.NewDecoder(file).Decode(cfg)
	if err != nil {
		log.Fatal("error reading config.json:", err)
	}

	if cfg.EnginePath == "" {
		cfg.EnginePath = "stockfish"
	}
	eng := NewEngine(cfg.EnginePath)

	engSet := cfg.EngineSettings
	if engSet.Memory != nil {
		eng.SetMemory(*engSet.Memory)
	}
	if engSet.Threads != nil {
		eng.SetThreads(*engSet.Threads)
	}
	eng.SetPrintAll(engSet.PrintAll)
	eng.SetPrintVarProgresss(engSet.PrintProgress)
	eng.Start()

	mvs := MovesFromString(cfg.VariationConfig.InitialMoves)

	fmt.Fprintln(os.Stderr, "Initial Starting Position:")
	eng.SetMoves(mvs)
	eng.write("d")
	eng.ReadUntilSubstring("Checkers:", func(s string) { fmt.Fprint(os.Stderr, s) })

	vars, err := eng.MakeVariations(
		mvs,
		cfg.VariationConfig.VariationDepth,
		cfg.VariationConfig.EngineDepth,
		cfg.VariationConfig.IsWhite,
	)

	if err != nil {
		log.Println("ERROR OCCURRED:", err)
		return
	}

	ofile, err := os.Open(cfg.OutputPath)
	if err != nil {
		log.Println("ERROR OCCURRED OPENING FILE:", err)
		return
	}
	defer ofile.Close()

	for moves := range vars {
		fmt.Fprintln(ofile, moves)
	}
}
