// 2019 Busang Inc.
// www.busanginc.com
// LS PLC와 통신하는 모듈. LS PLC사용설명서 XGT FEnet 국문 V2.1
package lsplc

import (
	"fmt"
	"strconv"
)

const LSPLC_HEADER_STRING string = "LSIS-XGT"

// 데이터타입
// 2Bytes Little Endian
const (
	LSPLC_DATATYPE_BIT = 0x0000
	LSPLC_DATATYPE_BYTE = 0x0001
	LSPLC_DATATYPE_WORD = 0x0002
	LSPLC_DATATYPE_DWORD = 0x0003
	LSPLC_DATATYPE_LWORD = 0x0004
	LSPLC_DATATYPE_CONTINUOUS = 0x0014
)

// 명령어
// 2Bytes Little Endian
const (
	LSPLC_COMMAND_REQUEST_READ = 0x0054
	LSPLC_COMMAND_RESPONSE_READ = 0x0055
	LSPLC_COMMAND_REQUEST_WRITE = 0x0058
	LSPLC_COMMAND_RESPONSE_WRITE = 0x0059
)

// PLC로 요청하는 메세지 프레임
// 각 멤버값들은 Big Endian임
type BSLSPlcRequestFrame struct {
	Command int16
	InvokeId int16
	DataType int16

	CommandPacket []byte
}

// PLC의 응답 데이터 1개
// Bool(X) %{P,M,L,K,F,T)X 데이터 크기=1Byte(최하위 비트)
// Word(W) %(P,M,L,K,F,T,C,D,S)W 데이터 크기=2Byte
type BSLSPlcResponseData struct {
	DataSize int16
	Data int
}

// PLC의 응답을 저장하는 프레임
// PLC의 워드응답은 Little Endian이지만 파싱된 구조체의 각 멤버값들은 Big Endian임
type BSLSPlcResponseFrame struct {
	Command int16
	InvokeId int16
	DataType int16

	// Todo. 이거는 나중에 처리해야겠음. 비트별로 PLC상태 알 수 있음.
	// 일단 들어오는 그대로 Little Endian으로 저장함.
	PLCInfo int16
	CPUInfo int8
	ModulePosition int8
	Reserved2BCC int8

	ErrorStatus int16
	ErrorNumber int16

	DataList []BSLSPlcResponseData
}

// PLC의 응답 프레임을 파싱하여 각 멤버변수에 담아줌.
func (frame *BSLSPlcResponseFrame)ReadFromBytes(bytes []byte) bool {
	var packetLen = int16(len(bytes))

	// 헤더의 길이는 20
	if packetLen < 20 {
		return false
	}

	var applicationInstructionLength = getInt16FromBigEndianWord(bytes[17], bytes[16])
	if (applicationInstructionLength + 20) != packetLen {
		return false
	}

	// Header 검사
	strTemp := string(bytes[0:8])

	if LSPLC_HEADER_STRING != strTemp {
		return false
	}

	// Source of Frame. Client->Server=0x33, Server->Client=0x11
	if bytes[13] != 0x11 {
		return false
	}

	frame.PLCInfo = getInt16FromBigEndianWord(bytes[10], bytes[11])
	frame.CPUInfo = int8(bytes[12])
	frame.InvokeId = getInt16FromBigEndianWord(bytes[15], bytes[14])
	frame.ModulePosition = int8(bytes[18])
	frame.Reserved2BCC = int8(bytes[19])
	// -- end of the Header -- //

	frame.Command = getInt16FromBigEndianWord(bytes[21], bytes[20])
	if frame.Command != LSPLC_COMMAND_RESPONSE_READ {
		return false
	}

	frame.DataType = getInt16FromBigEndianWord(bytes[23], bytes[22])
	frame.ErrorStatus = getInt16FromBigEndianWord(bytes[27], bytes[26])
	variablesCnt := getInt16FromBigEndianWord(bytes[29], bytes[28])
	if frame.ErrorStatus != 0x00{
		frame.ErrorNumber = variablesCnt
		return false
	}

	frame.DataList = make([]BSLSPlcResponseData, 0, variablesCnt)

	var startIndexForLen = 30
	for vIndex := 0; vIndex < int(variablesCnt); vIndex++ {
		varSize := int(getInt16FromBigEndianWord( bytes[startIndexForLen+1], bytes[startIndexForLen] ))

		startIndexForVar := startIndexForLen + 2
		var value int = 0
		for valIndex := 0; valIndex < varSize; valIndex++ {
			valTemp := int(bytes[startIndexForVar + valIndex])
			value += int(valTemp << (valIndex*8))
		}
		startIndexForLen = startIndexForVar + varSize

		frame.DataList = append(frame.DataList, BSLSPlcResponseData{DataSize: int16(varSize), Data: value})
	}

	return true
}

// 요청 프레임을 생성하여 CommandPacket에 담아줌. 주의!: 2바이트 이상은 Little Endian임
// invokeId: 식별자. 클라이언트 필요에 따라 활용
// dataType: LSPLC_DATATYPE_*
// varNames: Read 요청할 변수이름 명. 동시에 최대 16개까지 요청 할 수 있음
// 데이터타입은 Bool: %(P,M,L,K,F,T)X   Word: %(P,M,L,K,F,T,C,D,S)W
// 변수 이름은 16자 이내의 아스키값. 메뉴얼참조
func (frame *BSLSPlcRequestFrame)MakeReadCommand( invokeId int16, dataType int16,  varNames ...string ) bool {
	var varCnt int16 = int16(len(varNames))
	if varCnt < 1 || varCnt > 16{
		return false
	}

	frame.InvokeId = invokeId
	frame.DataType = dataType
	frame.Command = LSPLC_COMMAND_REQUEST_READ

	var bytes []byte = make([]byte, 8)

	bytes[1], bytes[0] = get2BytesFromInt(frame.Command) // Command
	bytes[3], bytes[2] = get2BytesFromInt(dataType) // Data Type
	bytes[5], bytes[4] = 0x00, 0x00 // Reserved
	bytes[7], bytes[6] = get2BytesFromInt(varCnt) // Number of variables

	for _, varName := range varNames {
		varNameLen := len(varName)
		b2, b1 := get2BytesFromInt(int16(varNameLen))
		bytes = append(bytes, b1, b2) // Length of Variable's name
		//fmt.Println(bytes)

		szTemp := []byte(varName)
		bytes = append(bytes, szTemp...) // Variable's name
	}

	lenOfData := len(bytes)
	header := makeHeader(0x01, int16(lenOfData))

	header = append(header, bytes...)
	bytes = header
	frame.CommandPacket = bytes

	return true
}

// 메세지 프레임 헤더 생성
func makeHeader(invokeId int16, dataLength int16) []byte {
	var bytes []byte = make([]byte, 20)

	strHeader := []byte(LSPLC_HEADER_STRING)
	copy(bytes, strHeader)// LSIS-XGT

	bytes[9], bytes[8] = 0x00, 0x00// Reserved
	bytes[11], bytes[10] = 0x00, 0x00 // PLC Info - Don't care
	bytes[12] = 0x00 // CPU Info - Don't care
	bytes[13] = 0x33 // Client -> Server
	bytes[15], bytes[14] = get2BytesFromInt(invokeId) // InvoleID
	bytes[17], bytes[16] = get2BytesFromInt(dataLength) // Length
	bytes[18] = 0x00 // FEnet Position - Don't care
	bytes[19] = 0x00 // BCC

	return bytes
}

// 테스트용 함수
func (frame *BSLSPlcRequestFrame) makeCommandTest(cmd int16, dataType int16) []byte{
	var bytes []byte = make([]byte, 8)

	const varCnt int16 = 1
	const varName string = "%DW102"
	const varNameLen = len(varName)
	fmt.Println("CMD:" + varName + "," + strconv.Itoa(varNameLen))

	bytes[1], bytes[0] = get2BytesFromInt(cmd) // Command
	bytes[3], bytes[2] = get2BytesFromInt(dataType) // Data Type
	bytes[5], bytes[4] = 0x00, 0x00 // Reserved
	bytes[7], bytes[6] = get2BytesFromInt(varCnt) // Number of variables
	//fmt.Println(bytes)

	b2, b1 := get2BytesFromInt(int16(varNameLen))
	bytes = append(bytes, b1, b2) // Length of Variable's name
	//fmt.Println(bytes)

	szTemp := []byte(varName)
	bytes = append(bytes, szTemp...) // Variable's name
	fmt.Print("Completed Application Instruction:")
	fmt.Println(bytes)

	lenOfData := len(bytes)
	header := makeHeader(0x01, int16(lenOfData))
	fmt.Print("Completed Header:")
	fmt.Println(header)

	header = append(header, bytes...)
	fmt.Print("Completed Command:")
	fmt.Println(header)
	bytes = header
	return bytes
}


// BigEndian
func get2BytesFromInt(val int16) (byte, byte){
	return uint8(val >> 8), uint8(val & 0xff)
}

// Little Endian to int16
func getInt16FromBigEndianWord(hByte byte, lByte byte) int16 {
	return int16(hByte<<8) + int16(lByte)
}
