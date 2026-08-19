package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/alterwalker/test_dating_bot/internal/seedgen"
)

func main() {
	count := flag.Int("count", 4920, "number of fictional profiles with narrow age range")
	wide := flag.Int("wide", 80, "extra profiles with wide age range 18-55")
	out := flag.String("out", "seed/fictional_profiles.json", "output path")
	prefixFlag := flag.String("prefix", "вымышленный_", "external_id prefix")
	seed := flag.Int64("seed", 42, "random seed for reproducible generation")
	flag.Parse()

	prefix := seedgen.NormalizePrefix(*prefixFlag)
	rng := seedgen.NewRNG(*seed)
	entries := seedgen.Generate(*count, *wide, prefix, rng)

	if err := seedgen.WriteFile(*out, entries); err != nil {
		log.Fatal(err)
	}

	info, _ := os.Stat(*out)
	fmt.Printf("wrote %d profiles (%d narrow + %d wide) to %s (%.1f MB)\n",
		len(entries), *count, *wide, *out, float64(info.Size())/(1024*1024))
}
