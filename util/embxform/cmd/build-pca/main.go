package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"

	"gonum.org/v1/gonum/mat"
)

type embedReq struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}
type embedResp struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func main() {
	var outPath, model, endpoint string
	var max int
	flag.StringVar(&outPath, "out", "/etc/chaosmith/pca_nomic_v15_768to1024.json", "output path for PCA json")
	flag.StringVar(&model, "model", "nomic-embed-text-v1.5", "embedding model")
	flag.StringVar(&endpoint, "endpoint", "http://127.0.0.1:1234/v1/embeddings", "embed endpoint")
	flag.IntVar(&max, "max", 50000, "max samples")
	flag.Parse()

	texts := readNDJSON(os.Stdin, max)
	if len(texts) == 0 {
		fmt.Fprintln(os.Stderr, "no input")
		os.Exit(1)
	}
	X := fetchEmbeddings(endpoint, model, texts)
	mean, comps := pca(X)
	// zero-extend comps to 1024 cols
	ext := make([][]float32, len(comps))
	for i := range comps {
		row := make([]float32, 1024)
		copy(row, comps[i])
		ext[i] = row
	}
	obj := map[string]any{"mean": mean, "components": ext}
	b, _ := json.Marshal(obj)
	if err := os.WriteFile(outPath, b, 0o644); err != nil {
		panic(err)
	}
	// derive id
	fmt.Println(deriveID(b))
}

func readNDJSON(r io.Reader, max int) []string {
	sc := bufio.NewScanner(r)
	var out []string
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err == nil {
			if t, _ := m["text"].(string); t != "" {
				out = append(out, t)
			}
		}
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out
}

func fetchEmbeddings(url, model string, texts []string) [][]float32 {
	const batch = 128
	var out [][]float32
	ctx := context.Background()
	for i := 0; i < len(texts); i += batch {
		j := i + batch
		if j > len(texts) {
			j = len(texts)
		}
		req := embedReq{Model: model, Input: texts[i:j]}
		b, _ := json.Marshal(req)
		httpReq, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(b))
		httpReq.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			panic(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			panic(fmt.Sprintf("embed http %d", resp.StatusCode))
		}
		var er embedResp
		if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
			panic(err)
		}
		for _, d := range er.Data {
			out = append(out, d.Embedding)
		}
	}
	return out
}

func pca(X [][]float32) ([]float32, [][]float32) {
	n := len(X)
	if n == 0 {
		return nil, nil
	}
	d := len(X[0])
	// compute mean
	mean := make([]float32, d)
	for _, row := range X {
		for i, v := range row {
			mean[i] += v
		}
	}
	for i := range mean {
		mean[i] /= float32(n)
	}
	// center matrix
	M := mat.NewDense(n, d, nil)
	for r := 0; r < n; r++ {
		for c := 0; c < d; c++ {
			M.Set(r, c, float64(X[r][c]-mean[c]))
		}
	}
	var svd mat.SVD
	ok := svd.Factorize(M, mat.SVDThin)
	if !ok {
		panic("svd failed")
	}
	var v mat.Dense
	svd.VTo(&v)
	vm := v.RawMatrix()
	cols := vm.Cols
	comps := make([][]float32, d)
	for i := 0; i < d; i++ {
		comps[i] = make([]float32, d)
		for j := 0; j < cols; j++ {
			comps[i][j] = float32(v.At(i, j))
		}
	}
	return mean, comps
}

func deriveID(b []byte) string {
	sum := sha256.Sum256(b)
	return "transform_id=pca-nomic-v1.5-768to1024@" + hex.EncodeToString(sum[:4])
}
