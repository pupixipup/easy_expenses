# Easy expenses
A simple full-stack app for converting pdf or image receipts to xlsx table.
This is the process:
1. Go server receives images or pdfs of receipts from the client
2. The receipts are analyzed with OpenAI API
3. XLSX table based on the analyzed data is created
4. Table and receipts get zipped and sent to a client as a response

## Requirements
### Backend
* Go 1.18+
* .env file with API_KEY for OpenAI
* Dependencies (install via go get):
  * [github.com/joho/godotenv](github.com/joho/godotenv)
  * [github.com/xuri/excelize/v2](github.com/xuri/excelize/v2)
### Frontend
* Node.js & NPM
## Get Started
### Backend
1. Set up .env file with `API_KEY`
2. Build an image with `docker build -t my-go-app .`
3. Run the image with `docker run -p 3333:3333 my-go-app`
### Frontend
1. Install deps with `npm i`
2. Run server with `npm run dev`