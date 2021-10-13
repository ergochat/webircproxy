// Copyright (c) 2021 Shivaram Lingamneni
// released under the MIT license

package irc

import (
	"fmt"
	"reflect"
	"testing"
	"unicode/utf8"
)

func assertEqual(found, expected interface{}) {
	if !reflect.DeepEqual(found, expected) {
		panic(fmt.Sprintf("expected %#v, found %#v", expected, found))
	}
}

func getTestingServer(chardet bool, encodings []string) *Server {
	config := new(Config)
	config.Transcoding.EnableChardet = chardet
	config.Transcoding.Encodings = encodings
	_, err := config.postprocessEncodings()
	if err != nil {
		panic(err)
	}
	server := new(Server)
	server.SetConfig(config)
	return server
}

func TestValidUnicode(t *testing.T) {
	server := getTestingServer(false, nil)

	assertEqual(server.transcodeToUTF8([]byte("PRIVMSG #ircv3 :hi there"), 512), []byte("PRIVMSG #ircv3 :hi there"))
	assertEqual(server.transcodeToUTF8([]byte("PRIVMSG #ircv3 :Привет"), 512), []byte("PRIVMSG #ircv3 :Привет"))
}

const (
	frlatin1          = "@msgid=t75wuypjy5j4yamj32gc4r2jqw;time=2021-10-13T05:27:37.293Z :slingamn!shivaram@example.com PRIVMSG #ircv3 :Le fromage est un aliment obtenu \xe0 partir de lait coagul\xe9, de produits laitiers ou d'\xe9l\xe9ments du lait comme le petit-lait ou la cr\xe8me. Le fromage est fabriqu\xe9 \xe0 partir de lait de vache principalement, mais aussi de brebis, de ch\xe8vre, de bufflonne ainsi qu'occasionnellement de chamelle, de renne, d'\xe9lan, de jument"
	frutf8            = "@msgid=t75wuypjy5j4yamj32gc4r2jqw;time=2021-10-13T05:27:37.293Z :slingamn!shivaram@example.com PRIVMSG #ircv3 :Le fromage est un aliment obtenu \xc3\xa0 partir de lait coagul\xc3\xa9, de produits laitiers ou d'\xc3\xa9l\xc3\xa9ments du lait comme le petit-lait ou la cr\xc3\xa8me. Le fromage est fabriqu\xc3\xa9 \xc3\xa0 partir de lait de vache principalement, mais aussi de brebis, de ch\xc3\xa8vre, de bufflonne ainsi qu'occasionnellement de chamelle, de renne, d'\xc3\xa9lan, de jument"
	frutf8replacement = "@msgid=t75wuypjy5j4yamj32gc4r2jqw;time=2021-10-13T05:27:37.293Z :slingamn!shivaram@example.com PRIVMSG #ircv3 :Le fromage est un aliment obtenu \xef\xbf\xbd partir de lait coagul\xef\xbf\xbd, de produits laitiers ou d'\xef\xbf\xbdl\xef\xbf\xbdments du lait comme le petit-lait ou la cr\xef\xbf\xbdme. Le fromage est fabriqu\xef\xbf\xbd \xef\xbf\xbd partir de lait de vache principalement, mais aussi de brebis, de ch\xef\xbf\xbdvre, de bufflonne ainsi qu'occasionnellement de chamelle, de renne, d'\xef\xbf\xbdlan, de jument"

	jautf8            = "PRIVMSG #ircv3 :ウイスキー（英: whisky、愛/米: whiskey）は、蒸留酒の一つで、大麦、ライ麦、トウモロコシなどの穀物を麦芽の酵素で糖化し、これをアルコール発酵させ蒸留したものである。"
	jashiftjis        = "PRIVMSG #ircv3 :\x83E\x83C\x83X\x83L\x81[\x81i\x89p: whisky\x81A\x88\xa4/\x95\xc4: whiskey\x81j\x82\xcd\x81A\x8f\xf6\x97\xaf\x8e\xf0\x82\xcc\x88\xea\x82\xc2\x82\xc5\x81A\x91\xe5\x94\x9e\x81A\x83\x89\x83C\x94\x9e\x81A\x83g\x83E\x83\x82\x83\x8d\x83R\x83V\x82\xc8\x82\xc7\x82\xcc\x8d\x92\x95\xa8\x82\xf0\x94\x9e\x89\xe8\x82\xcc\x8dy\x91f\x82\xc5\x93\x9c\x89\xbb\x82\xb5\x81A\x82\xb1\x82\xea\x82\xf0\x83A\x83\x8b\x83R\x81[\x83\x8b\x94\xad\x8dy\x82\xb3\x82\xb9\x8f\xf6\x97\xaf\x82\xb5\x82\xbd\x82\xe0\x82\xcc\x82\xc5\x82\xa0\x82\xe9\x81B"
	jautf8replacement = "PRIVMSG #ircv3 :\xef\xbf\xbdE\xef\xbf\xbdC\xef\xbf\xbdX\xef\xbf\xbdL\xef\xbf\xbd[\xef\xbf\xbdi\xef\xbf\xbdp: whisky\xef\xbf\xbdA\xef\xbf\xbd\xef\xbf\xbd/\xef\xbf\xbd\xef\xbf\xbd: whiskey\xef\xbf\xbdj\xef\xbf\xbd\xcd\x81A\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xcc\x88\xef\xbf\xbd\xc2\x82\xc5\x81A\xef\xbf\xbd\xe5\x94\x9e\xef\xbf\xbdA\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbdC\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbdA\xef\xbf\xbdg\xef\xbf\xbdE\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbdR\xef\xbf\xbdV\xef\xbf\xbd\xc8\x82\xc7\x82\xcc\x8d\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xf0\x94\x9e\x89\xef\xbf\xbd\xcc\x8dy\xef\xbf\xbdf\xef\xbf\xbd\xc5\x93\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbdA\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbdA\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbdR\xef\xbf\xbd[\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbdy\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbd\xcc\x82\xc5\x82\xef\xbf\xbd\xef\xbf\xbd\xef\xbf\xbdB"
)

func TestTestData(t *testing.T) {
	// sanity checks for test data
	assertEqual(utf8.ValidString(frutf8), true)
	assertEqual(utf8.ValidString(frlatin1), false)
	assertEqual(utf8.ValidString(frutf8replacement), true)

	assertEqual(utf8.ValidString(jautf8), true)
	assertEqual(utf8.ValidString(jashiftjis), false)
	assertEqual(utf8.ValidString(jautf8replacement), true)
}

func TestTranscodeWithFixedEncoding(t *testing.T) {
	server := getTestingServer(false, []string{"windows-1252"})
	assertEqual(string(server.transcodeToUTF8([]byte(frutf8), 512)), frutf8)
	assertEqual(string(server.transcodeToUTF8([]byte(frlatin1), 512)), frutf8)
}

func TestTranscodeWithFixedEncoding2(t *testing.T) {
	server := getTestingServer(false, []string{"Shift_JIS"})
	assertEqual(string(server.transcodeToUTF8([]byte(jautf8), 512)), jautf8)
	assertEqual(string(server.transcodeToUTF8([]byte(jashiftjis), 512)), jautf8)
}

func TestTranscodeWithChardet(t *testing.T) {
	server := getTestingServer(true, nil)
	assertEqual(string(server.transcodeToUTF8([]byte(frutf8), 512)), frutf8)
	assertEqual(string(server.transcodeToUTF8([]byte(frlatin1), 512)), frutf8)

	assertEqual(string(server.transcodeToUTF8([]byte(jautf8), 512)), jautf8)
	assertEqual(string(server.transcodeToUTF8([]byte(jashiftjis), 512)), jautf8)
}

func TestTranscodeWithUnicodeReplacementCharacter(t *testing.T) {
	server := getTestingServer(false, nil)
	assertEqual(string(server.transcodeToUTF8([]byte(frutf8), 512)), frutf8)
	assertEqual(string(server.transcodeToUTF8([]byte(frlatin1), 512)), frutf8replacement)

	assertEqual(string(server.transcodeToUTF8([]byte(jautf8), 512)), jautf8)
	// TODO get the python and go results to agree here
	//assertEqual(string(server.transcodeToUTF8([]byte(jashiftjis), 512)), jautf8replacement)
}

func BenchmarkTranscodeWithFixedEncoding(b *testing.B) {
	server := getTestingServer(false, []string{"windows-1252"})
	l1bytes := []byte(frlatin1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.transcodeToUTF8(l1bytes, 512)
	}
}

func BenchmarkTranscodeWithFixedEncoding2(b *testing.B) {
	server := getTestingServer(false, []string{"Shift_JIS"})
	shiftjisbytes := []byte(jashiftjis)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.transcodeToUTF8(shiftjisbytes, 512)
	}
}

func BenchmarkTranscodeWithChardet(b *testing.B) {
	server := getTestingServer(true, nil)
	l1bytes := []byte(frlatin1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.transcodeToUTF8(l1bytes, 512)
	}
}

func BenchmarkTranscodeWithChardet2(b *testing.B) {
	server := getTestingServer(true, nil)
	shiftjisbytes := []byte(jashiftjis)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.transcodeToUTF8(shiftjisbytes, 512)
	}
}

func BenchmarkTranscodeWithUnicodeReplacementCharacter(b *testing.B) {
	server := getTestingServer(false, nil)
	l1bytes := []byte(frlatin1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.transcodeToUTF8(l1bytes, 512)
	}
}

func BenchmarkTranscodeWithUnicodeReplacementCharacter2(b *testing.B) {
	server := getTestingServer(false, nil)
	shiftjisbytes := []byte(jashiftjis)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.transcodeToUTF8(shiftjisbytes, 512)
	}
}

func BenchmarkUTF8Validate(b *testing.B) {
	for i := 0; i < b.N; i++ {
		utf8.ValidString(frutf8)
	}
}
