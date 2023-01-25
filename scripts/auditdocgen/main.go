package main

import (
	"bytes"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/coder/coder/enterprise/audit"
)

func main() {
	auditableResourcesMap, err := readAuditableResources()
	if err != nil {
		log.Fatal("can't read auditableResources: ", err)
	}

	doc, err := readAuditDoc()
	if err != nil {
		log.Fatal("can't read audit doc: ", err)
	}

	doc, err = updateAuditDoc(doc, auditableResourcesMap)
	if err != nil {
		log.Fatal("can't update audit doc: ", err)
	}

	err = writeAuditDoc(doc)
	if err != nil {
		log.Fatal("can't write updated audit doc: ", err)
	}
}

type AuditableResourcesMap map[string]map[string]bool

func readAuditableResources() (AuditableResourcesMap, error) {
	auditableResourcesMap := make(AuditableResourcesMap)

	for resourceName, resourceFields := range audit.AuditableResources {
		friendlyResourceName := strings.Split(resourceName, ".")[2]
		fieldNameMap := make(map[string]bool)
		for fieldName, action := range resourceFields {
			fieldNameMap[fieldName] = action != audit.ActionIgnore
			auditableResourcesMap[friendlyResourceName] = fieldNameMap
		}
	}

	return auditableResourcesMap, nil
}

func readAuditDoc() ([]byte, error) {
	var doc []byte
	return doc, nil
}

func updateAuditDoc(doc []byte, auditableResourcesMap AuditableResourcesMap) ([]byte, error) {
	var updatedDoc []byte

	var buffer bytes.Buffer
	buffer.WriteByte('\n')
	buffer.WriteString("|<b>Resource<b>||\n")
	buffer.WriteString("|--|-----------------|\n")

	for resourceName, resourceFields := range auditableResourcesMap {

		buffer.Write([]byte("|" + resourceName + "|<table><thead><tr><th>Field</th><th>Tracked</th></tr></thead><tbody>"))

		for fieldName, isTracked := range resourceFields {
			buffer.Write([]byte("<tr><td>" + fieldName + "</td><td>" + strconv.FormatBool(isTracked) + "</td></tr>"))
		}

		buffer.WriteString("</tbody></table>\n")
	}

	fmt.Println("updated table", buffer.String())
	return updatedDoc, nil
}

func writeAuditDoc(doc []byte) error {
	return nil
}
