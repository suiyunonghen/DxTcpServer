unit Unit1;

interface

uses
  Winapi.Windows, Winapi.Messages, System.SysUtils, System.Variants, System.Classes, Vcl.Graphics,
  Vcl.Controls, Vcl.Forms, Vcl.Dialogs, OverbyteIcsWndControl,
  OverbyteIcsWSocket, Vcl.StdCtrls, Vcl.ExtCtrls,utils_buffer;

type
  TForm1 = class(TForm)
    Button1: TButton;
    Memo1: TMemo;
    Button2: TButton;
    Button3: TButton;
    Timer1: TTimer;
    Timer2: TTimer;
    procedure FormCreate(Sender: TObject);
    procedure Button1Click(Sender: TObject);
    procedure WSocket1DataAvailable(Sender: TObject; ErrCode: Word);
    procedure FormClose(Sender: TObject; var Action: TCloseAction);
    procedure Button3Click(Sender: TObject);
    procedure Timer2Timer(Sender: TObject);
  private
    { Private declarations }
  public
    { Public declarations }
    RStreamLst: TList;
    ridxLst: TList;
    Lst: TList;
  end;

var
  Form1: TForm1;

implementation

{$R *.dfm}

procedure TForm1.Button1Click(Sender: TObject);
var
  st: string;
  l: Word;
  tmpl:Word;
  WStream: TMemoryStream;
  idx: integer;
  WSocket1: TWSocket;
  i: Integer;
begin
  WStream := TMemoryStream.Create;
  for i := 0 to 1000 do
  begin
    Randomize;
    idx := Random(1000);
    WSocket1 := Lst.Items[idx];
    st := '不得闲测试按时发放阿斯蒂芬萨芬安抚萨芬按时发发萨芬阿斯蒂芬';
    l := Length(st)*sizeof(char);
    tmpl := WSocket_htons(l);
    WStream.WriteBuffer(tmpl,2);
    WStream.WriteBuffer(Pointer(st)^,l);
    WSocket1.Send(WStream.Memory,WStream.Size);
    WStream.Clear;
    Sleep(20);
    Application.ProcessMessages;
  end;
  WStream.Free;
end;

procedure TForm1.Button3Click(Sender: TObject);
begin
  Timer1.Enabled := not TImer1.Enabled;
  Timer2.Enabled := not Timer2.Enabled;
end;

procedure TForm1.FormClose(Sender: TObject; var Action: TCloseAction);
begin
  while Lst.Count> 0 do
  begin
    TWSocket(Lst.Items[Lst.Count - 1]).Close;
    Lst.Delete(Lst.Count - 1);
  end;
  while RStreamLst.Count>0 do
  begin
    TMemoryStream(RStreamLst.Items[RStreamLst.Count - 1]).Free;
    RStreamLst.Delete(RStreamLst.Count - 1);
  end;

  RStreamLst.Free;
  Lst.Free;

end;

procedure TForm1.FormCreate(Sender: TObject);
var
  WSocket: TWSocket;
  i: Integer;
  Stream: TMemoryStream;
begin
  Lst := TList.Create;
  ridxLst := TList.Create;
  RStreamLst := TList.Create;
  for i := 0 to 1000 do
  begin
    ridxLst.Add(nil);
    WSocket := TWSocket.Create(self);
    WSocket.Tag := i;
    WSocket.OnDataAvailable := WSocket1DataAvailable;
    lst.Add(WSocket);
    Stream := TMemoryStream.Create;
    RStreamLst.Add(Stream);

    WSocket.Addr := '127.0.0.1';
    WSocket.Port := '8340';
    WSocket.Connect;
  end;
end;

procedure TForm1.Timer2Timer(Sender: TObject);
var
  st: string;
  l: Word;
  tmpl:Word;
  WStream: TMemoryStream;
  idx: integer;
  WSocket1: TWSocket;
  i: Integer;
begin
  WStream := TMemoryStream.Create;
  for i := 0 to 150 do
  begin
    Randomize;
    idx := 500+Random(500);
    WSocket1 := Lst.Items[idx];
    st := '不得闲测试按时发放阿斯蒂芬萨芬安抚萨芬按时发发萨芬阿斯蒂芬';
    l := Length(st)*sizeof(char);
    tmpl := WSocket_htons(l);
    WStream.WriteBuffer(tmpl,2);
    WStream.WriteBuffer(Pointer(st)^,l);
    WSocket1.Send(WStream.Memory,WStream.Size);
    WStream.Clear;
  end;
  WStream.Free;
end;

procedure TForm1.WSocket1DataAvailable(Sender: TObject; ErrCode: Word);
var
  buf: array[0..1024] of Byte;
  L,rl: Word;
  lp: Integer;
  st: string;
  WSocket1: TWSocket;
  RStream: TMemoryStream;
  ridx: Integer;
begin
  WSocket1 := TWSocket(Sender);
  rl := WSocket1.Receive(@buf[0],1024);
  if rl = 0 then
    Exit;
  ridx := Integer(ridxLst.Items[WSocket1.Tag]);
  RStream := RStreamLst.Items[WSocket1.Tag];
  RStream.Position := RStream.Size;
  RStream.WriteBuffer(buf,rl);
  while True do
  begin
    RStream.Position := ridx;
    if RStream.Size - RStream.Position > 2 then
    begin
      RStream.ReadBuffer(l,2);
      l := WSocket_ntohs(l);
      if RStream.Size - RStream.Position < l then
        Break;
      //读取实际内容
      SetLength(st,l div 2);
      RStream.ReadBuffer(Pointer(st)^,l);
      RStream.Position := ridx + l + 2;
      Memo1.Lines.Add(Format('%d 号 Echo Recive：%s',[WSocket1.Tag,st]));
      ridx := RStream.Position;
      if RStream.Position = RStream.Size then
      begin
        ridx := 0;
        RStream.Position := 0;
      end;
      ridxLst.Items[WSocket1.Tag] := Pointer(ridx);
      if rl < 1024 then
        Break;
      rl := WSocket1.Receive(@buf[0],1024);
      if rl = 0 then
        Break;
    end
    else Break;

  end;
end;

end.
