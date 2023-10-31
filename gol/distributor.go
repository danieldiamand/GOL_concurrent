package gol

import (
	"fmt"
	"sync"
	"time"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels, keyPresses <-chan rune) {
	//Activate IO to output world:
	c.ioCommand <- ioInput
	c.ioFilename <- fmt.Sprintf("%dx%d", p.ImageHeight, p.ImageWidth)

	//Create 2D slice and store received world in it:
	startWorld := make([][]byte, p.ImageHeight)
	for y := 0; y < p.ImageHeight; y++ {
		startWorld[y] = make([]byte, p.ImageWidth)
		for x := 0; x < p.ImageWidth; x++ {
			startWorld[y][x] = <-c.ioInput
		}
	}

	worldChan := make(chan func(y, x int) byte, 1)
	safeWorld := makeSafeWorld(startWorld, p)
	worldChan <- safeWorld

	var wg sync.WaitGroup
	turn := 0
	timer := time.NewTimer(2 * time.Second)
	qPressed := false

	for turn < p.Turns {
		select {
		case safeWorld = <-worldChan:
			c.events <- TurnComplete{turn}
			worldChan <- safeWorld
			wg.Add(1)
			go distributeTurn(worldChan, p, &wg, c, turn)
			wg.Wait()
			turn++

		case <-timer.C:
			timer.Reset(2 * time.Second)
			safeWorld = <-worldChan
			c.events <- AliveCellsCount{turn, len(calculateAliveCells(safeWorld, p))}
			worldChan <- safeWorld
		case key := <-keyPresses:
			switch key {
			case 's':
				safeWorld = <-worldChan
				go sendWorldToPGM(safeWorld, turn, p, c)
				worldChan <- safeWorld
			case 'q':
				qPressed = true
			case 'p':
				println("Paused on turn", turn)
				for 'p' != <-keyPresses {
				}
				println("Continuing")
			}

		}
		if qPressed {
			break
		}

	}

	//Send final world to io
	safeWorld = <-worldChan
	sendWorldToPGM(safeWorld, turn, p, c)
	c.events <- FinalTurnComplete{turn, calculateAliveCells(safeWorld, p)}

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

// Makes a closure on a 2D slice with wrapped indexing
func makeSafeWorld(matrix [][]byte, p Params) func(y, x int) byte {
	return func(y, x int) byte {
		return matrix[(y+p.ImageHeight)%p.ImageHeight][(x+p.ImageWidth)%p.ImageWidth]
	}
}

// Divides up world from worldChan into number of threads and calls progressWorld on them, sends newWorld back down worldChan
func distributeTurn(worldChan chan func(y, x int) byte, p Params, wg *sync.WaitGroup, c distributorChannels, turn int) {
	oldWorld := <-worldChan

	//Create channels for each thread
	subWorlds := make([]chan [][]byte, p.Threads)
	for i := 0; i < p.Threads; i++ {
		subWorlds[i] = make(chan [][]byte)
	}

	//Divide up world and call progressWorld on each segment
	subHeight := p.ImageHeight / p.Threads
	for i := 0; i < p.Threads-1; i++ {
		startY := subHeight * i
		endY := subHeight * (i + 1)
		go progressWorld(oldWorld, subWorlds[i], p.ImageWidth, startY, endY, c, turn)
	}
	go progressWorld(oldWorld, subWorlds[p.Threads-1], p.ImageWidth, subHeight*(p.Threads-1), p.ImageHeight, c, turn)

	//Collect progressed world:
	var newWorld [][]byte
	for i := 0; i < p.Threads; i++ {
		newWorld = append(newWorld, <-subWorlds[i]...)
	}

	worldChan <- makeSafeWorld(newWorld, p)
	wg.Done()
}

// Progresses section of world and sends results down out
func progressWorld(oldWorld func(y, x int) byte, out chan<- [][]byte, width, startY, endY int, c distributorChannels, turn int) {
	//Make newWorld
	newWorld := make([][]byte, endY-startY)
	for y := 0; y < endY-startY; y++ {
		newWorld[y] = make([]byte, width)
	}

	//Calculate contents of newWorld
	for y := startY; y < endY; y++ {
		for x := 0; x < width; x++ {
			liveNeighbours := countNeighbours(oldWorld, y, x)
			if oldWorld(y, x) == 255 { //if cells alive:
				if liveNeighbours == 2 || liveNeighbours == 3 { //any live cell with two or three live neighbours is unaffected
					newWorld[y-startY][x] = 255
				} else { //cell dies, (slices are init to 0 so don't need to write to newWorld)
					c.events <- CellFlipped{turn, util.Cell{x, y}}
				}
			} else { //cells dead
				if liveNeighbours == 3 { //any dead cell with exactly three live neighbours becomes alive
					newWorld[y-startY][x] = 255
					c.events <- CellFlipped{turn, util.Cell{x, y}}
				}
			}
		}
	}

	out <- newWorld
}

//Returns the number of alive neighbours of a given cell
func countNeighbours(board func(y, x int) byte, y, x int) int8 {
	var count int8 = 0
	if board(y+1, x+1) == 255 {
		count++
	}
	if board(y+1, x) == 255 {
		count++
	}
	if board(y+1, x-1) == 255 {
		count++
	}
	if board(y, x+1) == 255 {
		count++
	}
	if board(y, x-1) == 255 {
		count++
	}
	if board(y-1, x+1) == 255 {
		count++
	}
	if board(y-1, x) == 255 {
		count++
	}
	if board(y-1, x-1) == 255 {
		count++
	}

	return count
}

//Returns list of all alive cells in board
func calculateAliveCells(board func(y, x int) byte, p Params) []util.Cell {
	cells := make([]util.Cell, 0)
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			if board(y, x) == 255 {
				cells = append(cells, util.Cell{x, y})
			}
		}
	}
	return cells
}

//Prepares io for output and sends board down it a pixel at a time
func sendWorldToPGM(world func(y, x int) uint8, turn int, p Params, c distributorChannels) {
	c.ioCommand <- ioOutput
	c.ioFilename <- fmt.Sprintf("%dx%dx%d", p.ImageHeight, p.ImageWidth, turn)
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			c.ioOutput <- world(y, x)
		}
	}
}
