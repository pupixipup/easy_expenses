import { FormEvent, useState } from 'react'
import './App.css'

const URL = "http://localhost:3333"
function App() {
   // State to store the selected files
   const [files, setFiles] = useState<File[]>([]);
   const [base64, setBase64] = useState<string>("");

   // Handle file input change
   const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
     // Use the spread operator to keep previous files and add new ones
     if (e.target.files) {
       setFiles([...e.target.files]);
      }
   };
 
   // Handle form submission
   const handleSubmit = async (e: FormEvent) => {
     e.preventDefault();
     const formData = new FormData();
 
     // Append each file to the FormData object
     files.forEach((file, index) => {
       formData.append(`file${index}`, file);
     });
 
     // Log the FormData object for now (You can send it to the server)
     const res = await fetch(URL, {
      method: "POST",
      body: formData,
      headers: {
        //  must be automatically set to 'multipart/form-data'
      },
     })

    setBase64(await res.text())
    };
 
   return (
     <div>
       <form onSubmit={handleSubmit}>
         <input
           accept=".pdf, .png, .jpg, .jpeg"
           type="file" 
           multiple 
           onChange={handleFileChange} 
           />
         <button disabled={files.length === 0} type="submit">Upload</button>
       </form>
       {/* Display selected files */}
       <ul>
         {files.length > 0 && files.map((file, index) => (
           <li key={index}>{file.name}</li>
          ))}
       </ul>
          { base64 !== "" && <img style={{maxWidth: "200px"}} src={base64} />}
     </div>
   );
}

export default App
