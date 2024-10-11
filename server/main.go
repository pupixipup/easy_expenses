package main

// TODO: Make XLSX table
// TODO: Zip all files & send zip to user
// TODO: Error handling with status codes

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	// "github.com/xuri/excelize/v2"
	"os/exec"

	"github.com/joho/godotenv"
	"github.com/xuri/excelize/v2"
)

var categories = []string{"Mobile Phone", "Transport", "Gym card or wellness"}

type Global struct {
	dirname string
}

type ReceiptInfo struct {
	CompanyName string `json:"company_name"`
	Date        string `json:"date"`
	Cost        int    `json:"cost"`
	Category    string `json:"category"`
	path        string
}

var global Global

func main() {
	// f := excelize.NewFile()
	dirname, err := os.MkdirTemp("", "uploads")
	if err != nil {
		fmt.Println("Error creating temp dir")
		return
	}
	global.dirname = dirname
	err = godotenv.Load()
	if err != nil {
		fmt.Println("Error loading .env file")
		return
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

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// return

	err := r.ParseMultipartForm(10 << 20) // 10MB
	if err != nil {
		fmt.Println("Could not parse")
		http.Error(w, "Could not parse multipart form", http.StatusBadRequest)
		return
	}
	form := r.MultipartForm
	files := form.File

	str := ""

	channel := make(chan ReceiptInfo, 10)
	// var channels []chan ReceiptInfo

	for _, fileHeaders := range files {
		for _, fileHeader := range fileHeaders {
			go func() {
				savedFile, err := saveFile(fileHeader)
				var base64String string
				// PDF TO IMG ---------------------------
				if savedFile.contentType == "application/pdf" {
					outPath := filepath.Join(global.dirname, "output.jpg")
					savedFile.extension = "jpg"
					cmd := exec.Command("convert",
						"-verbose",
						"-density", "150",
						"-trim",
						savedFile.path+"[0]",
						"-quality", "100",
						"-flatten",
						"-sharpen", "0x1.0",
						outPath,
					)
					// Run the command and capture any errors
					err := cmd.Run()
					if err != nil {
						fmt.Println(err)
					}

					file, err := os.Open(outPath)
					if err != nil {
						http.Error(w, "Error when opening image", http.StatusInternalServerError)
						return
					}
					fileBytes, err := io.ReadAll(file)
					file.Close()
					if err != nil {
						fmt.Println(err, "1")
						http.Error(w, "Error when encoding image", http.StatusInternalServerError)
						return
					}

					base64String = base64.StdEncoding.EncodeToString(fileBytes)

					fmt.Println("PDF successfully converted to PNG!")
				} else {
					savedFile.fetchedFile.Seek(0, 0)
					bytes, err := io.ReadAll(savedFile.fetchedFile)
					if err != nil {
						fmt.Println(err, "2")
						http.Error(w, "Error when encoding image", http.StatusInternalServerError)
						return
					}
					base64String = base64.StdEncoding.EncodeToString(bytes)
				}

				str = "data:image/" + savedFile.extension + ";base64," + base64String

				// Analyze image
				receiptInfo, fetchErr := fetchData(base64String)
				if err != nil {
					http.Error(w, fetchErr.Error(), http.StatusInternalServerError)
					return
				}
				receiptInfo.path = savedFile.path
				channel <- *receiptInfo
				savedFile.savedFile.Close()
				savedFile.fetchedFile.Close()
			}()
		}
	}

	var receipts []ReceiptInfo

	for _, fileHeaders := range files {
		for range fileHeaders {
			info := <-channel
			receipts = append(receipts, info)
		}
	}

	tableName, err := makeTable(receipts)
	if err != nil {
		http.Error(w, "Cant create xlsx table", http.StatusInternalServerError)
	}
	path, err := makeZip(receipts, tableName)
	if err != nil {
		http.Error(w, "Cant create zip", http.StatusInternalServerError)
	}
	fmt.Println(path)
	fmt.Println(receipts, "Receipts")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(str))
}

type UploadResponse struct {
	Object   string `json:"object"`
	Filename string `json:"filename"`
	Status   string `json:"status"`
	Id       string `json:"id"`
}

func makeZip(receipts []ReceiptInfo, tablePath string) (string, error) {
	// Create a new zip file
	pathZip := "output.zip"
	zipFile, err := os.Create("output.zip")
	if err != nil {
		return "", err
	}
	defer zipFile.Close()

	// Create a new zip writer
	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Add receipts
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
	// Add table
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

	fmt.Println("Successfully created zip file with a file inside!")
	return pathZip, nil
}

func makeTable(receipts []ReceiptInfo) (string, error) {
	filePath := "Book1.xlsx"
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Println(err)
		}
	}()
	// Set value of a cell.
	f.SetCellValue("Sheet1", "A2", "Utläggsräkning")
	f.SetCellValue("Sheet1", "A3", "Namn:")
	f.SetCellValue("Sheet1", "B3", "Romas Bitinas")

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

	// Set active sheet of the workbook.
	// Save spreadsheet by the given path.
	if err := f.SaveAs(filePath); err != nil {
		fmt.Println(err)
	}
	return filePath, nil
}

func fetchData(base64 string) (*ReceiptInfo, error) {

	// Create the request payload
	payload := map[string]interface{}{
		"model": "gpt-4o-mini",
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": `Return JSON with the following format: {cost: number, company_name: string, category: string, date: string}.
						"cost" is total cost to be paid, based on the receipt.
						"company_name" is a name of the company.
						"date" is a date of the receipt.
						Date should be formatted as "DD-MM-YYYY"
						"category" belongs to enum ["SL Card", "Mobile", "Gym or wellness"]
						If you can't identify any value, set it to empty string for strings,
						And to zero for numbers
						`,
					},
					{
						"type": "image_url",
						"image_url": map[string]interface{}{
							"url": "data:image/jpg;base64," + base64,
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

// Type representing the overall structure
type CompletionResponse struct {
	Choices           []Choice `json:"choices"`
	Created           float64  `json:"created"`
	ID                string   `json:"id"`
	Model             string   `json:"model"`
	Object            string   `json:"object"`
	SystemFingerprint string   `json:"system_fingerprint"`
	Usage             Usage    `json:"usage"`
}

// Nested type representing choices
type Choice struct {
	FinishReason string  `json:"finish_reason"`
	Index        int     `json:"index"`
	Logprobs     *string `json:"logprobs"` // Since logprobs is <nil> in your JSON, using a pointer here
	Message      Message `json:"message"`
}

// Nested type representing the message field
type Message struct {
	Content string  `json:"content"`
	Refusal *string `json:"refusal"` // Since refusal is <nil> in your JSON, using a pointer
	Role    string  `json:"role"`
}

// Nested type representing the usage field
type Usage struct {
	CompletionTokens        int               `json:"completion_tokens"`
	CompletionTokensDetails CompletionDetails `json:"completion_tokens_details"`
	PromptTokens            int               `json:"prompt_tokens"`
	PromptTokensDetails     PromptDetails     `json:"prompt_tokens_details"`
	TotalTokens             int               `json:"total_tokens"`
}

// Nested type representing completion tokens details
type CompletionDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

// Nested type representing prompt tokens details
type PromptDetails struct {
	CachedTokens int `json:"cached_tokens"`
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

	// Create the destination file on the server
	path := filepath.Join(global.dirname, fileHeader.Filename)
	extension := strings.Replace(filepath.Ext(fileHeader.Filename), ".", "", 1)
	dst, err := os.Create(path)

	if err != nil {
		return nil, &ResError{
			fmt.Sprintf("Error creating file on server %s", fileHeader.Filename),
			http.StatusInternalServerError,
		}
	}

	_, err = io.Copy(dst, file)
	// Read the file content
	if err != nil {
		return nil, &ResError{
			"Unable to read file",
			http.StatusInternalServerError,
		}
	}

	// Encode the file content to Base64
	// str = base64String
	// Copy the uploaded file data to the server's file
	_, err = io.Copy(dst, file)
	fmt.Println(contentType)
	return &FileSave{
		path,
		extension,
		contentType,
		file,
		dst,
	}, nil
}
