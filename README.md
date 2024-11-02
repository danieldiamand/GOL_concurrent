# Concurrent Conway's Game of Life

This project is a concurrent implementation of Conway's Game of Life, completed as part of coursework at the University of Bristol. The application reads a starting board from a PGM image file, simulates cell evolution over time, and allows control over grid dimensions and threading.

## Features
- Concurrent cell evolution based on Conway's Game of Life rules.

## Usage
Run the program with the following flags:

- `-w <width>`: Set the width of the board.
- `-h <height>`: Set the height of the board.
- `-t <threads>`: Specify the number of threads to use.

### Example
Navigate to route directory of the project and run:
```bash
./conway -w 512 -h 512 -t 8
```

<em> Note: The program requires a matching PGM image file in `./images` for the specified width and height. If no image is found, it will not start. </em>
