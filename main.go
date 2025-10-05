package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	ErrMatchNotFound    = errors.New("match not found")
	ErrMismatchBoards   = errors.New("mismatched boards")
	ErrMismatchedStarts = errors.New("mismatched starts")
)

var Rx = regexp.MustCompile(`^([A-Za-z,_\-{}'\(\) ]+)(\d+) ([0-9 ]+); ([0-9 \-]+);.*board ([0-9 ]+);.*p12 ([0-9 ]+);`)

// player, oppenent_id
type Match struct {
	Name  string
	Opp   string
	Score int
	Table int
	Start int
	Round int
}

type Matches map[string]Match

func parseFirstLastName(name string) string {
	res := strings.Split(name, ",")
	switch len(res) {
	case 1:
		return name
	case 2:
		name = strings.TrimLeft(res[1]+" "+res[0], " ")
		return name
	default:
		return ""
	}

}

// parseResults turns tsh results row into match data
func parseResults(id int, s string, mm Matches) Matches {
	res := Rx.FindStringSubmatch(s)

	// Null rows should be ignored
	if len(res) == 0 {
		return mm
	}

	// Ensure we have exactly 7 values parsed
	if len(res) != 7 {
		fmt.Println("parsing results has failed")
		os.Exit(1)
	}

	// Parse values into map structure
	name := strings.TrimRight(res[1], " ")
	// If name contains a comma "Mackay(GM), Lewis" then reverse it
	name = parseFirstLastName(name)

	oppos := strings.Fields(res[3])
	scores := strings.Fields(res[4])
	boards := strings.Fields(res[5])
	starts := strings.Fields(res[6])

	var key string
	var m Match
	for i, o := range oppos {
		round := i + 1
		key = createKey(round, id)
		m = Match{
			Name:  name,
			Round: round,
			Opp:   o,
		}
		mm[key] = m
	}

	for i, s := range scores {
		round := i + 1
		key = createKey(round, id)
		m = mm[key]
		score, _ := strconv.Atoi(s)
		m.Score = score
		mm[key] = m
	}

	for i, b := range boards {
		round := i + 1
		key = createKey(round, id)
		m = mm[key]
		table, _ := strconv.Atoi(b)
		m.Table = table
		mm[key] = m
	}

	for i, st := range starts {
		round := i + 1
		key = createKey(round, id)
		m = mm[key]
		start, _ := strconv.Atoi(st)
		m.Start = start
		mm[key] = m
	}

	return mm
}

type Result struct {
	Division string
	Round    int
	Player1  string
	Score1   int
	Player2  string
	Score2   int
}

// countMaxRoundsPlayers parses the match map and returns the max round and player
func countMaxRoundsPlayers(mm Matches) (int, int) {
	var nr, np int

	// Round/player count from 1, not zero
	for k := range mm {
		// Split "{round}-{player}"
		rp := strings.Split(k, "-")

		r, err := strconv.Atoi(rp[0])
		if err != nil {
			log.Fatal(err)
		}

		p, err := strconv.Atoi(rp[1])
		if err != nil {
			log.Fatal(err)
		}

		// nr is max number of rounds
		if r > nr {
			nr = r
		}

		// np is max number of players
		if p > np {
			np = p
		}
	}

	return nr, np
}

// createKey returns a stringified key from round and player ids
func createKey(r int, p int) string {
	// round-player
	return fmt.Sprintf("%d-%d", r, p)
}

// fetchMatch looks up a match by key
func fetchMatch(key string, mm Matches) (Match, error) {
	m, ok := mm[key]
	if !ok {
		return Match{}, ErrMatchNotFound
	}
	return m, nil
}

// validateBoards ensures the board number for each player matches
func validateBoards(b1, b2 int) error {
	if b1 == b2 {
		return nil
	}
	return ErrMismatchBoards
}

// validateStarts ensures one player starts and the other replies
func validateStarts(s1, s2 int) error {
	if s1 == 1 && s2 == 2 {
		return nil
	}
	if s1 == 2 && s2 == 1 {
		return nil
	}
	return ErrMismatchedStarts
}

// formatResult creates a result structure for the match and respecs start/reply
func formatResult(m1 Match, m2 Match, div string) Result {
	// if player1 is replying then swap player order
	if m1.Start == 2 {
		m1, m2 = m2, m1
	}

	return Result{
		Division: div,
		Round:    m1.Round,
		Player1:  m1.Name,
		Score1:   m1.Score,
		Player2:  m2.Name,
		Score2:   m2.Score,
	}
}

// processMatches validates the pairings in each round and returns a slice of csv results
func processMatches(mm Matches, div string) []Result {
	// parse number of rounds and players from the match map
	nr, np := countMaxRoundsPlayers(mm)

	seen := make(map[string]bool)

	var results []Result

	// iterate over rounds / players - count from 1 not zero
	for r := 1; r < nr+1; r++ {
		var byes_count int
		var match_count int
		// iterate over all players
		for p := 1; p < np+1; p++ {
			// Lookup match data for player1
			key1 := createKey(r, p)

			// skip if we've seen them before
			if _, ok := seen[key1]; ok {
				continue
			}

			m1, err := fetchMatch(key1, mm)
			if err != nil {
				log.Fatalf("failed to match %s, %v", key1, err)
			}

			seen[key1] = true

			// lookup the opponent data
			opp, _ := strconv.Atoi(m1.Opp)
			if opp == 0 {
				// this represents a bye
				byes_count++
				continue
			}
			key2 := createKey(r, opp)

			// Skip any player seen before
			if _, ok := seen[key2]; ok {
				continue
			}

			m2, err := fetchMatch(key2, mm)
			if err != nil {
				log.Fatal(err)
			}

			// increment match counter for this round
			match_count++

			seen[key2] = true

			// Validate that the boards match
			if err := validateBoards(m1.Table, m2.Table); err != nil {
				log.Fatal(err)
			}

			// Validate starts and replies are consistent
			if err := validateStarts(m1.Start, m2.Start); err != nil {
				log.Fatal(err)
			}

			// Add to results slice
			result := formatResult(m1, m2, div)
			results = append(results, result)
		}

		// matches found plus byes (divided by two since we match each player separately) should equal half the number of players
		if 2*match_count+byes_count-np != 0 {
			fmt.Printf("Round %d, found %d matches and %d byes but expected %d\n", r, match_count, byes_count/2, np/2)
		}
	}
	return results
}

// printCSV prints results in CSV format
func printCSV(results []Result) {
	for _, r := range results {
		fmt.Printf("%s,%d,%s,%d,%s,%d\n", r.Division, r.Round, r.Player1, r.Score1, r.Player2, r.Score2)
	}
}

// process reads the data and passes it to processMatches
func process(file string) {
	// Create a map to store the match data for each player
	matches := make(Matches, 100)

	// Read the file as bytes
	b, err := os.ReadFile(file)
	if err != nil {
		log.Fatal(err)
	}

	// Parse b slice to lines of string data
	lines := strings.Split(string(b), "\n")
	for id, line := range lines {
		matches = parseResults(id+1, line, matches)
	}

	// Parse the division name from the file name
	division := strings.ToUpper(strings.TrimSuffix(file, ".t"))

	// processMatches
	results := processMatches(matches, division)

	printCSV(results)

}

// parseFiles returns list of files from arguments or .t files in the directory
func parseFiles(args []string) ([]string, error) {
	if len(args) > 1 {
		return args[1:], nil
	}

	files, err := filepath.Glob("*.t")
	return files, err
}

func main() {
	files, err := parseFiles(os.Args)
	if err != nil {
		log.Fatal(err)
	}

	for _, file := range files {
		process(file)
	}
}
