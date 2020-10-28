package main

import (
	"context"
	"fmt"
	"github.com/mailgun/mailgun-go"
	"github.com/nguyenthenguyen/docx"
	"github.com/spf13/viper"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type Details struct {
	ClaimDate string
	Email     string
	Subject   string
	Name      string
	Message   string
}

func main() {

	viper.SetConfigFile(".env")
	viper.AutomaticEnv()
	err := viper.ReadInConfig()

	if err != nil {
		log.Fatalf("Error while reading config file %s", err)
	}

	tmpl := template.Must(template.ParseFiles("templates/form.html"))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tmpl.Execute(w, nil)
			return
		}

		details := Details{
			ClaimDate: r.FormValue("claim_date"),
			Email:     r.FormValue("email"),
			Subject:   r.FormValue("subject"),
			Name:      r.FormValue("name"),
			Message:   r.FormValue("message"),
		}

		evidenceFilePath, dirName, errUploadFile := uploadFile(r, details)
		if errUploadFile != nil {
			log.Println(errUploadFile)
			fmt.Fprintf(w, errUploadFile.Error())
			return
		}

		formFilePath, errCreateDoc := createDoc(dirName, details)
		if errCreateDoc != nil {
			log.Println(err)
			fmt.Fprintf(w, err.Error())
			return
		}

		_, errEmail := sendEmailMessage(formFilePath, evidenceFilePath)
		if errEmail != nil {
			log.Println(errEmail)
			fmt.Fprintf(w, errEmail.Error())
			return
		}

		_ = details

		tmpl.Execute(w, struct{ Success bool }{true})
	})

	http.ListenAndServe(":3000", nil)
}

func uploadFile(r *http.Request, details Details) (string, string, error) {
	fmt.Println("File Upload Endpoint Hit")

	// Parse our multipart form, 10 << 20 specifies a maximum
	// upload of 10 MB files.
	r.ParseMultipartForm(10 << 20)
	// FormFile returns the first file for the given key `myFile`
	// it also returns the FileHeader so we can get the Filename,
	// the Header and the size of the file
	file, handler, err := r.FormFile("file")
	if err != nil {
		return "", "Error Retrieving the File", err
	}
	defer file.Close()
	log.Printf("Uploaded File: %+v\n", handler.Filename)
	log.Printf("File Size: %+v\n", handler.Size)
	log.Printf("MIME Header: %+v\n", handler.Header)

	fileName := createFileName(details.Name, details.ClaimDate, handler.Filename)

	dirName, errDir := ioutil.TempDir("./", details.Name)
	if errDir != nil {
		return "", "", errDir
	}
	dirName = createFolderName(dirName)

	filePath := dirName + "/" + fileName
	dst, err := os.Create(filePath)
	if err != nil {
		return "", "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		return "", "", err
	}

	return filePath, dirName, nil
}

func createDoc(dirName string, details Details) (string, error) {
	r, err := docx.ReadDocxFile("./Wellness Reimbursement Form.docx")
	if err != nil {
		return "", err
	}
	docx1 := r.Editable()

	docx1.Replace("placeholder_name", details.Name, -1)

	claimDate, _ := time.Parse(time.RFC3339, details.ClaimDate)

	docx1.Replace("placeholder_claim_date", fmt.Sprintf("%s/%d", claimDate.Month().String(), claimDate.Year()), -1)

	fileName := createFileName(details.Name, details.ClaimDate, "Wellness Reimbursement Form.docx")
	filePath := dirName + "/" + fileName

	docx1.WriteToFile(filePath)

	r.Close()

	return filePath, nil
}

func createFileName(userName string, date string, filename string) string {
	fileName := strings.ToLower(userName + "_" + date + "_" + filename)
	return strings.Replace(fileName, " ", "_", -1)
}

func createFolderName(userName string, ) string {
	dirName := strings.ToLower(userName)
	return strings.Replace(dirName, " ", "_", -1)
}

func sendEmailMessage(form string, evidence string) (string, error) {

	// Create an instance of the Mailgun Client
	mg := mailgun.NewMailgun(viper.GetString("DOMAIN"), viper.GetString("APIKEY"))

	sender := "sender@example.com"
	subject := "Fancy subject!"
	body := "Hello from Mailgun Go!"
	recipient := viper.GetString("RECIPIENT")
	// The message object allows you to add attachments and Bcc recipients
	message := mg.NewMessage(sender, subject, body, recipient)
	message.AddAttachment(form)
	message.AddAttachment(evidence)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	// Send the message with a 10 second timeout
	resp, id, err := mg.Send(ctx, message)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("ID: %s Resp: %s\n", id, resp)

	return id, err

}
