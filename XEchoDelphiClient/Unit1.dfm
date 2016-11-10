object Form1: TForm1
  Left = 0
  Top = 0
  Caption = 'Form1'
  ClientHeight = 361
  ClientWidth = 767
  Color = clBtnFace
  Font.Charset = DEFAULT_CHARSET
  Font.Color = clWindowText
  Font.Height = -11
  Font.Name = 'Tahoma'
  Font.Style = []
  OldCreateOrder = False
  OnClose = FormClose
  OnCreate = FormCreate
  PixelsPerInch = 96
  TextHeight = 13
  object Button1: TButton
    Left = 8
    Top = 0
    Width = 75
    Height = 25
    Caption = 'Echo'
    TabOrder = 0
    OnClick = Button1Click
  end
  object Memo1: TMemo
    Left = 0
    Top = 64
    Width = 767
    Height = 297
    Align = alBottom
    Anchors = [akLeft, akTop, akRight, akBottom]
    Lines.Strings = (
      'Memo1')
    ScrollBars = ssBoth
    TabOrder = 1
  end
  object Button2: TButton
    Left = 248
    Top = 0
    Width = 75
    Height = 25
    Caption = 'Button2'
    TabOrder = 2
  end
  object Button3: TButton
    Left = 136
    Top = 1
    Width = 75
    Height = 25
    Caption = #24320#22987
    TabOrder = 3
    OnClick = Button3Click
  end
  object Timer1: TTimer
    Enabled = False
    Interval = 500
    OnTimer = Button1Click
    Left = 208
    Top = 112
  end
  object Timer2: TTimer
    Enabled = False
    Interval = 300
    OnTimer = Timer2Timer
    Left = 344
    Top = 96
  end
end
