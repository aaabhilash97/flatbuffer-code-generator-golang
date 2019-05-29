package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/iancoleman/strcase"
)

type mappingVal struct {
	IsScalar bool
	Type     string
}

var typeMapping = map[string]mappingVal{
	"long":    mappingVal{true, "int64"},
	"int":     mappingVal{true, "int32"},
	"float":   mappingVal{true, "float32"},
	"double":  mappingVal{true, "float64"},
	"bool":    mappingVal{true, "bool"},
	"string":  mappingVal{false, "string"},
	"[ubyte]": mappingVal{false, "[]byte"},
}

// Field -
type Field struct {
	Name string
	Type string
}

// Table -
type Table struct {
	Name   string
	Fields []*Field
}

// Fbs -
type Fbs struct {
	InputFile  string
	OutputFile string
	SourceName string
	Package    string
	NameSpace  string
	RootType   string
	Tables     []*Table
}

func main() {
	var input string
	flag.StringVar(&input, "i", "", "Input fbs file")
	var outputF string
	flag.StringVar(&outputF, "o", "", "output fbs folder")
	flag.Parse()

	if input == "" {
		log.Fatal("Input is required")
	}
	if outputF == "" {
		log.Fatal("output folder is required")
	}
	fName := path.Base(input)

	output := &Fbs{}

	output.InputFile = input
	output.OutputFile = path.Join(outputF, strings.ReplaceAll(fName, ".fbs", ".fb.go"))
	output.SourceName = fName

	file, err := os.Open(input)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)

	var pendingTable *Table

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if startsWith(line, "//") {
			continue
		}

		namespace, ok := findNameSpace(line)
		if ok {
			output.NameSpace = namespace
			nameSpaceSplit := strings.Split(namespace, ".")
			output.Package = nameSpaceSplit[len(nameSpaceSplit)-1]
			continue
		}

		table, ok := findTable(line)
		if ok {
			if table == "Pagination" {
				pendingTable = nil
				continue
			}
			pendingTable = &Table{}
			pendingTable.Name = table
			output.Tables = append(output.Tables, pendingTable)
			continue
		}

		rootType, ok := findRootType(line)
		if ok {
			output.RootType = rootType
			continue
		}

		fieldName, fieldType, ok := findField(line)
		if ok && pendingTable != nil {
			field := &Field{
				Name: fieldName,
				Type: fieldType,
			}
			pendingTable.Fields = append(pendingTable.Fields, field)
			continue
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
	output.OutputFile = path.Join(outputF, output.NameSpace, strings.ReplaceAll(fName, ".fbs", ".fb.go"))
	generateGoCode(*output)
}

func findTable(line string) (string, bool) {
	tableMatch, _ := regexp.Compile("table\\s+(\\w+)\\s?\\{")
	foundTableMatch := tableMatch.FindStringSubmatch(line)
	if len(foundTableMatch) > 1 {
		return foundTableMatch[1], true
	}
	return "", false
}

func findField(line string) (field, fieldType string, ok bool) {
	matcher, _ := regexp.Compile("(\\w+)\\s?\\:\\s?([\\[\\]\\w]+)\\s?\\;")
	foundMatch := matcher.FindStringSubmatch(line)
	if len(foundMatch) > 2 {
		return foundMatch[1], foundMatch[2], true
	}
	return "", "", false
}

func findNameSpace(line string) (string, bool) {
	matcher, _ := regexp.Compile("namespace\\s+(.+)\\;")
	foundMatch := matcher.FindStringSubmatch(line)
	if len(foundMatch) > 1 {
		return foundMatch[1], true
	}
	return "", false
}

func findRootType(line string) (string, bool) {
	matcher, _ := regexp.Compile("root_type\\s+(\\w+)\\;")
	foundMatch := matcher.FindStringSubmatch(line)
	if len(foundMatch) > 1 {
		return foundMatch[1], true
	}
	return "", false
}

func startsWith(input, substr string) bool {
	if len(substr) > len(input) {
		return false
	}
	if input[0:len(substr)] == substr {
		return true
	}
	return false
}

func generateGoCode(fbs Fbs) {
	f, err := os.Create(fbs.OutputFile)
	if err != nil {
		log.Fatal(err)
	}

	defer f.Close()
	output := ""
	output += fmt.Sprintf("// Code generated by flat-code-gen-fo. DO NOT EDIT.\n// source: %s\n\n", fbs.SourceName)

	packageString := fmt.Sprintf("package %s\n\n", fbs.Package)
	output += packageString

	importString := fmt.Sprintf("import (\n\tflatbuffers \"github.com/google/flatbuffers/go\"\n)\n\n")
	output += importString

	for _, table := range fbs.Tables {
		s := fmt.Sprintf("type X%s struct {\n", strcase.ToCamel(table.Name))
		output += s
		for _, field := range table.Fields {
			s := fmt.Sprintf("\t%s %s\n", strcase.ToCamel(field.Name), typeMapping[field.Type].Type)
			output += s
		}
		output += "}\n"

		output += fmt.Sprintf("// Create to build flat buf binary - %s\n", strcase.ToCamel(table.Name))
		output += fmt.Sprintf("func (value *X%s) Create() []byte {\n", strcase.ToCamel(table.Name))
		output += "builder := flatbuffers.NewBuilder(1024)\n"
		for _, field := range table.Fields {
			if typeMapping[field.Type].Type == "string" {
				output += fmt.Sprintf("%s := builder.CreateString(value.%s)\n", strcase.ToCamel(field.Name), strcase.ToCamel(field.Name))
			} else if typeMapping[field.Type].Type == "[]byte" {
				output += fmt.Sprintf("%sStart%sVector(builder, len(value.%s))\n", strcase.ToCamel(table.Name), strcase.ToCamel(field.Name), strcase.ToCamel(field.Name))
				output += fmt.Sprintf("for i := len(value.%s); i >= 0; i-- {\n", strcase.ToCamel(field.Name))
				output += fmt.Sprintf("builder.PrependByte(byte(i))\n}\n")
				output += fmt.Sprintf("%s := builder.EndVector(len(value.%s))\n", field.Name, strcase.ToCamel(field.Name))
			}
		}
		output += fmt.Sprintf("%sStart(builder)\n", strcase.ToCamel(table.Name))
		for _, field := range table.Fields {
			if typeMapping[field.Type].IsScalar {
				output += fmt.Sprintf("%sAdd%s(builder, value.%s)\n", strcase.ToCamel(table.Name), strcase.ToCamel(field.Name), strcase.ToCamel(field.Name))
			} else if typeMapping[field.Type].Type == "string" {
				output += fmt.Sprintf("%sAdd%s(builder, %s)\n", strcase.ToCamel(table.Name), strcase.ToCamel(field.Name), strcase.ToCamel(field.Name))
			} else if typeMapping[field.Type].Type == "[]byte" {
				output += fmt.Sprintf("%sAdd%s(builder, %s)\n", strcase.ToCamel(table.Name), strcase.ToCamel(field.Name), field.Name)
			}
		}
		output += fmt.Sprintf("new%s := %sEnd(builder)\n", strcase.ToCamel(table.Name), strcase.ToCamel(table.Name))
		output += fmt.Sprintf("builder.Finish(new%s)\n", strcase.ToCamel(table.Name))
		output += "buf := builder.FinishedBytes()\n"
		output += "return buf\n"
		output += "}\n\n"

		output += fmt.Sprintf("// Read to Read %s from bytes\n", strcase.ToCamel(table.Name))
		output += fmt.Sprintf("func (value *X%s) Read(buf []byte) *X%s {\n", strcase.ToCamel(table.Name), strcase.ToCamel(table.Name))
		output += fmt.Sprintf("new%s := GetRootAs%s(buf, 0)\n", strcase.ToCamel(table.Name), strcase.ToCamel(table.Name))
		output += fmt.Sprintf("if new%s == nil {\nreturn nil\n}\n", strcase.ToCamel(table.Name))
		for _, field := range table.Fields {
			if typeMapping[field.Type].IsScalar {
				output += fmt.Sprintf("value.%s = new%s.%s()\n", strcase.ToCamel(field.Name), strcase.ToCamel(table.Name), strcase.ToCamel(field.Name))
			} else if typeMapping[field.Type].Type == "string" {
				output += fmt.Sprintf("value.%s = string(new%s.%s())\n", strcase.ToCamel(field.Name), strcase.ToCamel(table.Name), strcase.ToCamel(field.Name))
			}
		}
		output += "return value\n"
		output += "}\n"
	}
	outputByte, _ := format.Source([]byte(output))
	f.Write(outputByte)
}
