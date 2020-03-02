package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func main() {
	inputDir := os.Args[1]
	outputDir := os.Args[2]
	log.Printf("gathering test programs from %s to generate test inputs in %s", inputDir, outputDir)

	files, err := ioutil.ReadDir(inputDir)
	if err != nil {
		log.Fatal("failed to read %s: %s", inputDir, err)
	}

	var resultBaseAddrs = make(map[string]uint32)

	const suffix = ".elf"
	for _, info := range files {
		if info.IsDir() {
			continue
		}
		filename := info.Name()
		if !strings.HasSuffix(filename, suffix) {
			continue
		}
		elfFilePath := filepath.Join(inputDir, filename)
		name := filename[:len(filename)-len(suffix)]
		log.Printf("working on %s from %s", name, elfFilePath)
		sigFilePath := filepath.Join(inputDir, name+".signature.output")

		name = strings.ReplaceAll(name, "-", "_") // make a valid identifier
		binFilePath := filepath.Join(outputDir, name+".bin")
		wantFilePath := filepath.Join(outputDir, name+".want")

		err := genBinFile(elfFilePath, binFilePath)
		if err != nil {
			log.Printf("failed to generate %s from %s: %s", binFilePath, elfFilePath, err)
			if err, ok := err.(*exec.ExitError); ok && len(err.Stderr) != 0 {
				log.Printf("stderr from objcopy:\n%s", err.Stderr)
			}
			continue
		}

		syms, err := gatherSymbols(elfFilePath)
		if err != nil {
			log.Printf("failed to gather symbols from %s: %s", elfFilePath, err)
			continue
		}
		counts := resultCounts(syms)

		results, err := gatherResultValues(sigFilePath)
		if err != nil {
			log.Printf("failed to gather result values from %s: %s", sigFilePath, err)
			continue
		}

		w, err := os.Create(wantFilePath)
		if err != nil {
			log.Printf("failed to create %s: %s", wantFilePath, err)
			continue
		}
		resultIdx := 0
		for _, count := range counts {
			for i := 0; i < count; i++ {
				v := results[resultIdx]
				resultIdx++
				fmt.Fprintf(w, "%08x\n", v)
			}
			w.WriteString("---\n")
		}

		resultBaseAddrs[name] = syms["begin_signature"]
	}

	var names []string
	for name := range resultBaseAddrs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		addr := resultBaseAddrs[name]
		fmt.Printf("rv32case!(%s, 0x%08x);\n", name, addr)
	}
}

func genBinFile(elfFilePath, binFilePath string) error {
	cmd := exec.Command("riscv32-unknown-elf-objcopy", "-O", "binary", elfFilePath, binFilePath)
	return cmd.Run()
}

func gatherSymbols(elfFilePath string) (map[string]uint32, error) {
	ret := make(map[string]uint32)
	cmd := exec.Command("riscv32-unknown-elf-objdump", "-t", elfFilePath)
	r, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 5 {
			continue
		}
		if fields[2] != ".data" {
			continue
		}
		v, err := strconv.ParseUint(fields[0], 16, 32)
		if err != nil {
			continue
		}
		name := fields[len(fields)-1]
		ret[name] = uint32(v)
	}

	err = cmd.Wait()
	if err != nil {
		return nil, err
	}

	return ret, sc.Err()
}

func gatherResultValues(fn string) ([]uint32, error) {
	r, err := os.Open(fn)
	if err != nil {
		return nil, err
	}

	var ret []uint32
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		v, err := strconv.ParseUint(sc.Text(), 16, 32)
		if err != nil {
			return nil, err
		}
		ret = append(ret, uint32(v))
	}
	return ret, sc.Err()

}

func resultCounts(syms map[string]uint32) []int {
	var ret []int
	start := syms["begin_signature"]
	for i := 1; ; i++ {
		end, ok := syms[fmt.Sprintf("test_%d_res", i)]
		if !ok {
			end = syms["end_signature"]
			ret = append(ret, int(end-start)/4)
			break
		}
		ret = append(ret, int(end-start)/4)
		start = end
	}
	return ret
}
