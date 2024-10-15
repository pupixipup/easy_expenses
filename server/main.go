package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"github.com/xuri/excelize/v2"
)

const dirname = "uploads"

var mu = sync.Mutex{}

type ReceiptInfo struct {
	CompanyName string `json:"company_name"`
	Date        string `json:"date"`
	Cost        int    `json:"cost"`
	RawCost     int    `json:"raw_cost_text"`
	Category    string `json:"category"`
	path        string
}

type CompletionResponse struct {
	Choices []Choice `json:"choices"`
	Created float64  `json:"created"`
	ID      string   `json:"id"`
}

type Choice struct {
	FinishReason string  `json:"finish_reason"`
	Index        int     `json:"index"`
	Logprobs     *string `json:"logprobs"`
	Message      Message `json:"message"`
}

type Message struct {
	Content string  `json:"content"`
	Refusal *string `json:"refusal"`
	Role    string  `json:"role"`
}

type ResError struct {
	message string
	code    int
}

type FileSave struct {
	path        string
	extension   string
	contentType string
	fetchedFile multipart.File
	savedFile   *os.File
}

func main() {
	err := os.Mkdir(dirname, 0755)
	if err != nil {
		panic("Error creating temp dir")
	}
	err = godotenv.Load()
	if err != nil {
		panic("Error loading .env file")
	}

	http.HandleFunc("/", uploadFile)

	err = http.ListenAndServe(":3333", nil)
	if err != nil {
		panic(err)
	}
}

func uploadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// Playground settings (Not intended to be used other than locally)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	err := r.ParseMultipartForm(10 << 20) // 10MB
	if err != nil {
		http.Error(w, "Could not parse multipart form", http.StatusBadRequest)
		return
	}
	form := r.MultipartForm
	files := form.File

	userName := ""
	userNameValue, ok := form.Value["name"]
	if !ok || len(userNameValue) < 1 {
		userName = "Your name"
	} else {
		userName = userNameValue[0]
	}

	var wg sync.WaitGroup
	chanSize := 0

	var receipts []ReceiptInfo
	ctx, cancel := context.WithCancel(context.Background())

	// Channel to capture the first error
	errorChan := make(chan *ResError, 1)
	for _, fileHeaders := range files {
		for _, fileHeader := range fileHeaders {
			chanSize++
			wg.Add(1)
			go processReceipt(fileHeader, errorChan, &wg, &receipts)
		}
	}

	go func() {
		wg.Wait()
		cancel()
	}()

	select {
	case err := <-errorChan:
		http.Error(w, err.message, err.code)
		return
	case <-ctx.Done():
	}

	fmt.Println("All receipts: ", receipts)

	tablePath, err := makeTable(receipts, userName)
	if err != nil {
		http.Error(w, "Cant create xlsx table", http.StatusInternalServerError)
		return
	}
	pathZip, err := makeZip(receipts, tablePath)
	if err != nil {
		fmt.Println(err, "error1")
		http.Error(w, "Cant create zip", http.StatusInternalServerError)
		return
	}
	http.ServeFile(w, r, pathZip)

	// Wipe the stuff
	os.Remove(pathZip)
	os.Remove(tablePath)
	os.RemoveAll(dirname)
	os.Mkdir(dirname, 0755)
}

type UploadResponse struct {
	Object   string `json:"object"`
	Filename string `json:"filename"`
	Status   string `json:"status"`
	Id       string `json:"id"`
}

func makeZip(receipts []ReceiptInfo, tablePath string) (string, error) {
	// Create a new zip file
	zipFile, err := os.Create("output.zip")
	if err != nil {
		return "", err
	}
	defer zipFile.Close()

	// Create a new zip writer
	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Add receipts
	fmt.Println(receipts, "Receipts")
	for _, receipt := range receipts {
		file, err := os.Open(receipt.path)
		name := filepath.Base(receipt.path)
		if err != nil {
			return "", err
		}
		fileWriter, err := zipWriter.Create(name)
		if err != nil {
			return "", err
		}
		_, err = io.Copy(fileWriter, file)
		if err != nil {
			return "", err
		}
		file.Close()
	}
	file, err := os.Open(tablePath)
	if err != nil {
		return "", err
	}
	name := filepath.Base(tablePath)
	fileWriter, err := zipWriter.Create(name)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(fileWriter, file)
	file.Close()

	return zipFile.Name(), nil
}

func makeTable(receipts []ReceiptInfo, name string) (string, error) {
	filePath := "expenses_" + getMonthYear() + ".xlsx"
	f := excelize.NewFile()

	f.SetCellValue("Sheet1", "A2", "Utläggsräkning")
	f.SetCellValue("Sheet1", "A3", "Namn:")
	f.SetCellValue("Sheet1", "B3", name)

	f.SetCellValue("Sheet1", "A5", "Bolag")
	f.SetCellValue("Sheet1", "B5", "Type")
	f.SetCellValue("Sheet1", "C5", "Date")
	f.SetCellValue("Sheet1", "D5", "Cost")
	index := 6
	for i, receipt := range receipts {
		cellNum := strconv.Itoa(i + index)
		f.SetCellValue("Sheet1", "A"+cellNum, receipt.CompanyName)
		f.SetCellValue("Sheet1", "B"+cellNum, receipt.Category)
		f.SetCellValue("Sheet1", "C"+cellNum, receipt.Date)
		f.SetCellValue("Sheet1", "D"+cellNum, receipt.Cost)
	}

	if err := f.SaveAs(filePath); err != nil {
		return "", err
	}
	err := f.Close()
	return filePath, err
}

func fetchData(base64 string, dataType string) (*ReceiptInfo, error) {
	// Create the request payload
	payload := map[string]interface{}{
		"model": "gpt-4o", // OR gpt-4o-mini
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": `
						You are given an image of a receipt. It can be either in Swedish or English. Analyze the receipt image thoroughly and 
						return JSON with the following format: 
						{
							company_name: string,
							cost: number,
							raw_cost_text: string,
							category: string,
							date: string
						}
						- "company_name" is a name of the company.
						- "cost" is total cost (sometimes has currency such SEK or kr and often labeled with "Totalt", "Summa", or "Att betala"). The cost is important, please pay attention to it.
						- "raw_cost_text" is the exact text you see for the cost before parsing it into a number.
						- "date" is a date of the receipt. Date should be formatted as "DD-MM-YYYY".
						- "category" belongs to enum ["SL Card", "Mobile", "Fitness"].

						If you can't identify any value, set it to an empty string for strings, and to zero for numbers. However, please look meticulously, as the receipt usually contains all the fields.

						Examples of possible cost formats: "150.00", "1.234,56", "2 345,67 kr", "500 SEK", "150:-"
						`,
					},
					{
						"type": "image_url",
						"image_url": map[string]interface{}{
							"url": "data:" + dataType + ";base64," + base64,
						},
					},
				},
			},
		},
		"response_format": map[string]interface{}{
			"type": "json_object",
		},
		"max_tokens": 300,
	}

	// Convert the payload to JSON
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("Error marshaling JSON: %v\n", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("Error creating request: %v\n", err)
	}
	apiKey := os.Getenv("API_KEY")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Error sending request: %v\n", err)
	}
	defer resp.Body.Close()

	// Read and print the response
	if resp.StatusCode == http.StatusOK {
		var result CompletionResponse // ResponseGPT
		err = json.NewDecoder(resp.Body).Decode(&result)
		res := result.Choices[0].Message.Content

		var receiptInfo ReceiptInfo
		json.Unmarshal([]byte(res), &receiptInfo)
		if err != nil {
			return nil, err
		}
		return &receiptInfo, nil
	}
	return nil, fmt.Errorf("Error: %s\n", resp.Status)
}

func saveFile(fileHeader *multipart.FileHeader) (*FileSave, *ResError) {
	// Open the file
	contentType := fileHeader.Header.Get("Content-Type")
	file, err := fileHeader.Open()
	if err != nil {
		return nil, &ResError{
			fmt.Sprintf("Error opening file %s", fileHeader.Filename),
			http.StatusInternalServerError,
		}
	}
	defer file.Close()

	path := filepath.Join(dirname, fileHeader.Filename)
	extension := strings.Replace(filepath.Ext(fileHeader.Filename), ".", "", 1)
	dst, err := os.Create(path)

	if err != nil {
		return nil, &ResError{
			fmt.Sprintf("Error creating file on server %s", fileHeader.Filename),
			http.StatusInternalServerError,
		}
	}

	_, err = io.Copy(dst, file)
	if err != nil {
		return nil, &ResError{
			"Unable to read file",
			http.StatusInternalServerError,
		}
	}

	_, err = io.Copy(dst, file)
	return &FileSave{
		path,
		extension,
		contentType,
		file,
		dst,
	}, nil
}

func getMonthYear() string {
	currentTime := time.Now()

	month := currentTime.Format("Jan")
	monthLower := month[:3]

	year := currentTime.Format("2006")

	return fmt.Sprintf("%s%s", monthLower, year)
}

func pdfToJpg(path string) (string, error) {
	outPath := filepath.Join(dirname, "output.jpg")
	cmd := exec.Command("convert",
		"-verbose",
		"-density", "150",
		"-trim",
		path+"[0]",
		"-quality", "100",
		"-flatten",
		"-sharpen", "0x1.0",
		outPath,
	)
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	return outPath, nil
}

func fileToBase64(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", nil
	}
	fileBytes, err := io.ReadAll(file)
	file.Close()
	return base64.StdEncoding.EncodeToString(fileBytes), nil
}

func processReceipt(fileHeader *multipart.FileHeader, errorChan chan *ResError, wg *sync.WaitGroup, receipts *[]ReceiptInfo) {
	savedFile, err := saveFile(fileHeader)

	if err != nil {
		errorChan <- err
		return
	}
	var base64String string
	if savedFile.contentType == "application/pdf" {
		outPath, err := pdfToJpg(savedFile.path)
		if err != nil {
			errorChan <- &ResError{
				"Error during PDF handling",
				http.StatusInternalServerError,
			}
			return
		}
		savedFile.extension = "jpg"
		savedFile.contentType = "image/jpg"
		encodedStr, err := fileToBase64(outPath)
		if err != nil {
			errorChan <- &ResError{
				"Error during base64 conversion",
				http.StatusInternalServerError,
			}
			return
		}
		base64String = encodedStr
	} else {
		savedFile.fetchedFile.Seek(0, 0)
		bytes, err := io.ReadAll(savedFile.fetchedFile)
		if err != nil {
			errorChan <- &ResError{
				"Error when encoding image",
				http.StatusInternalServerError,
			}
			return
		}
		base64String = base64.StdEncoding.EncodeToString(bytes)
	}

	// Analyze image
	receiptInfo, fetchErr := fetchData(base64String, savedFile.contentType)

	if fetchErr != nil {
		errorChan <- &ResError{
			"Could not fetch from openAI API",
			http.StatusInternalServerError,
		}
		return
	}
	savedFile.savedFile.Close()
	savedFile.fetchedFile.Close()

	receiptInfo.path = savedFile.path
	mu.Lock()
	*receipts = append(*receipts, *receiptInfo)
	mu.Unlock()
	wg.Done()
}
