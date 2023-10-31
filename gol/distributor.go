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
func distributor(p Params, c distributorChannels) {

	// Send things down channels to start
	c.ioCommand <- ioInput
	c.ioFilename <- fmt.Sprintf("%dx%d", p.ImageHeight, p.ImageWidth)

	// Create a 2D slice to store the world.

	world := make([][]byte, p.ImageHeight)
	for y := 0; y < p.ImageHeight; y++ {
		world[y] = make([]byte, p.ImageWidth)
		for x := 0; x < p.ImageWidth; x++ {
			world[y][x] = <-c.ioInput
		}
	}

	immutableWorld := makeImmutableMatrix(p, world)
	immutableWorldChannel := make(chan func(y, x int) byte, 1)
	immutableWorldChannel <- immutableWorld

	turn := 0

	var wg sync.WaitGroup

	timer := time.NewTimer(2 * time.Second)
	for turn < p.Turns {
		select {
		case immutableWorld = <-immutableWorldChannel:
			turn++
			immutableWorldChannel <- immutableWorld
			wg.Add(1)
			go distributeTurn(immutableWorldChannel, p, &wg)
			wg.Wait()
			c.events <- TurnComplete{turn}
		case <-timer.C:
			timer.Reset(2 * time.Second)
			immutableWorld = <-immutableWorldChannel
			c.events <- AliveCellsCount{turn, len(calculateAliveCells(p, immutableWorld))}
			immutableWorldChannel <- immutableWorld

		}

	}

	// TODO: Report the final state using FinalTurnCompleteEvent.
	c.events <- FinalTurnComplete{turn, calculateAliveCells(p, <-immutableWorldChannel)}

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

func distributeTurn(immutableBoardChannel chan func(y, x int) uint8, p Params, wg *sync.WaitGroup) {
	// Execute all turns of the Game of Life.
	channels := make([]chan [][]byte, p.Threads)
	for i := 0; i < p.Threads; i++ {
		channels[i] = make(chan [][]byte)
	}

	subHeight := p.ImageHeight / p.Threads
	immutableWorld := <-immutableBoardChannel
	var world [][]byte
	for i := 0; i < p.Threads-1; i++ {
		startY := subHeight * i
		endY := subHeight * (i + 1)
		go golWorker(p.ImageWidth, startY, endY, immutableWorld, channels[i])
	}
	go golWorker(p.ImageWidth, subHeight*(p.Threads-1), p.ImageHeight, immutableWorld, channels[p.Threads-1])

	for i := 0; i < p.Threads; i++ {
		world = append(world, <-channels[i]...)
	}
	wg.Done()
	immutableBoardChannel <- makeImmutableMatrix(p, world)
}

// makeImmutableMatrix takes an existing 2D matrix and wraps it in a getter closure.
func makeImmutableMatrix(p Params, matrix [][]uint8) func(y, x int) uint8 {
	return func(y, x int) uint8 {
		return matrix[(y+p.ImageHeight)%p.ImageHeight][(x+p.ImageWidth)%p.ImageWidth]
	}
}

func golWorker(width, startY, endY int, oldBoard func(y, x int) uint8, out chan<- [][]byte) {
	out <- calculateNextState(width, startY, endY, oldBoard)
}

//using indexing x,y where 0,0 is top left of board
func calculateNextState(width, startY, endY int, oldBoard func(y, x int) uint8) [][]byte {
	//make new world
	newWorld := make([][]byte, endY-startY)
	for y := 0; y < endY-startY; y++ {
		newWorld[y] = make([]byte, width)
	}

	//update new world
	for y := startY; y < endY; y++ {
		for x := 0; x < width; x++ {
			count := liveNeighbourCount(y, x, width, oldBoard)
			if oldBoard(y, x) == 255 { //if cells alive:
				if count == 2 || count == 3 { //any live cell with two or three live neighbours is unaffected
					newWorld[y-startY][x] = 255
				}
				//any live cell with fewer than two or more than three live neighbours dies
				//in go slices are initialized to zero, so we don't need to do anything
			} else { //cells dead
				if count == 3 { //any dead cell with exactly three live neighbours becomes alive
					newWorld[y-startY][x] = 255
				}
			}
		}
	}
	return newWorld
}

func liveNeighbourCount(y, x, width int, board func(y, x int) uint8) int8 {
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

func calculateAliveCells(p Params, board func(y, x int) uint8) []util.Cell {
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

//
//func boardToPGM(board func(y, x int) uint8){
//	for y := 0; y < p.ImageHeight; y++ {
//		world[y] = make([]byte, p.ImageWidth)
//		for x := 0; x < p.ImageWidth; x++ {
//			world[y][x] = <-c.ioInput
//		}
//	}
//}
