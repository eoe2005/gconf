package main

var ResMap map[string][]byte = map[string][]byte{
	"/static/test.js" : []byte("var asda js"),

}

func GetContent(key string) []byte {
	if content,ok := ResMap[key];ok{
		return content
	}
	return []byte("")
}

