import { FormEvent, useState } from 'react'
import './App.css'

const URL = "http://localhost:3333"
function App() {
   // State to store the selected files
   const [files, setFiles] = useState<File[]>([]);
  const [name, setName] = useState("");
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

     formData.append("name", name)
 
     // Log the FormData object for now (You can send it to the server)
     const res = await fetch(URL, {
      method: "POST",
      body: formData,
      headers: {
        //  must be automatically set to 'multipart/form-data'
      },
     })
     const blob = await res.blob()
     const url = window.URL.createObjectURL(blob);
     const a = document.createElement('a');
     a.href = url;
     const months = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December'];
     const date = new Date();
     const currMonth = months[date.getMonth()].toLowerCase().slice(0, 3)
     const currYear = date.getFullYear()
     a.download = `expenses_${currMonth}${currYear}.zip`;
     // Append to the document and trigger download
     document.body.appendChild(a);
     a.click();
      
      // Clean up
    setTimeout(() => {
      document.body.removeChild(a);
      window.URL.revokeObjectURL(url);
    }, 100);
    // setBase64(await res.text())
    };
 
   return (
     <div>
       <form className="form" onSubmit={handleSubmit}>
        <input placeholder="Name" type="text" value={name} onChange={(e) => setName(e.target.value)}/>
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
     </div>
   );
}

export default App
