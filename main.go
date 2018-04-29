package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	placeArg := flag.String("place", "", "Search place (shortcut or full ftp directory path)")
	fromArg := flag.String("from", "00000000", "Lower date limit (YYYYmmdd format)")
	toArg := flag.String("to", "99999999", "Upper date limit (YYYYmmdd format)")
	patternsArg := flag.String("patterns", "", "Search patterns (split with comma)")

	flag.Parse()

	if *placeArg == "" {
		fmt.Fprintln(os.Stderr, "Usage:")
		flag.PrintDefaults()
		os.Exit(1)
	}
	if *patternsArg == "" {
		fmt.Fprintln(os.Stderr, "Usage:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	var directory string
	switch *placeArg {
	case "contract":
		directory = "/fcs_regions/Tatarstan_Resp/contracts/"
	case "notification":
		directory = "/fcs_regions/Tatarstan_Resp/notifications/"
	case "schedule":
		directory = "/fcs_regions/Tatarstan_Resp/plan_schedules/"
	case "purchase":
		directory = "/fcs_regions/Tatarstan_Resp/plan_purchases/"
	default:
		directory = *placeArg
	}

	searchParams := SearchParams{
		Directory: directory,
		FromDate:  *fromArg,
		ToDate:    *toArg,
		Patterns:  strings.Split(*patternsArg, ","),
	}

	for result := range Search(&searchParams) {
		fmt.Printf("Found %s in %s\n", result.Match, result.XmlName)
	}
}
