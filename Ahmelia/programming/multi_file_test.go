package programming

import (
	"fmt"
	"strings"
	"testing"
)

type recordingOps struct {
	files map[string]string
	calls []string
	failOn string
}

func newRecordingOps() *recordingOps {
	return &recordingOps{files: map[string]string{}}
}
func (o *recordingOps) check(f SourceFile) error {
	if f.ProjectID != "p" { return fmt.Errorf("project mismatch") }
	if o.failOn != "" && f.Path == o.failOn { return fmt.Errorf("forced failure") }
	return nil
}
func (o *recordingOps) Create(f SourceFile, c string) (SourceFileResult,error) {
	if err:=o.check(f); err!=nil{return SourceFileResult{},err}
	if _,ok:=o.files[f.Path];ok{return SourceFileResult{},fmt.Errorf("exists")}
	o.files[f.Path]=c; o.calls=append(o.calls,"create:"+f.Path)
	return SourceFileResult{File:f,After:c,Content:c},nil
}
func (o *recordingOps) Read(f SourceFile)(SourceFileResult,error){ c,ok:=o.files[f.Path];if !ok{return SourceFileResult{},fmt.Errorf("missing")};return SourceFileResult{File:f,Content:c,After:c},nil}
func (o *recordingOps) Write(f SourceFile,c string)(SourceFileResult,error){if err:=o.check(f);err!=nil{return SourceFileResult{},err};b:=o.files[f.Path];o.files[f.Path]=c;o.calls=append(o.calls,"write:"+f.Path);return SourceFileResult{File:f,Before:b,After:c,Content:c},nil}
func (o *recordingOps) Edit(f SourceFile,a,b string)(SourceFileResult,error){if err:=o.check(f);err!=nil{return SourceFileResult{},err};old:=o.files[f.Path];if !strings.Contains(old,a){return SourceFileResult{},fmt.Errorf("target missing")};after:=strings.Replace(old,a,b,1);o.files[f.Path]=after;o.calls=append(o.calls,"edit:"+f.Path);return SourceFileResult{File:f,Before:old,After:after,Content:after},nil}
func (o *recordingOps) Move(f SourceFile,n string)(SourceFileResult,error){if err:=o.check(f);err!=nil{return SourceFileResult{},err};c,ok:=o.files[f.Path];if !ok{return SourceFileResult{},fmt.Errorf("missing")};if _,ok:=o.files[n];ok{return SourceFileResult{},fmt.Errorf("dest exists")};delete(o.files,f.Path);o.files[n]=c;o.calls=append(o.calls,"move:"+f.Path+"->"+n);return SourceFileResult{File:f,Before:c,After:c,Content:c},nil}
func (o *recordingOps) Rename(f SourceFile,n string)(SourceFileResult,error){return o.Move(f,n)}
func (o *recordingOps) Delete(f SourceFile)(SourceFileResult,error){if err:=o.check(f);err!=nil{return SourceFileResult{},err};c,ok:=o.files[f.Path];if !ok{return SourceFileResult{},fmt.Errorf("missing")};delete(o.files,f.Path);o.calls=append(o.calls,"delete:"+f.Path);return SourceFileResult{File:f,Before:c},nil}

func sf(path string) SourceFile { return SourceFile{Path:path,Language:"go",ProjectID:"p"} }

func TestMultiFileEditor_AppliesDeterministically(t *testing.T) {
	o:=newRecordingOps()
	e,_:=NewMultiFileEditor(o)
	p:=MultiFilePlan{ID:"p1",Project:CodeProject{ID:"p",Root:t.TempDir()},Changes:[]MultiFileChange{
		{ID:"2",Operation:SourceCreate,File:sf("b.go"),Content:"package p"},
		{ID:"1",Operation:SourceCreate,File:sf("a.go"),Content:"package p"},
	}}
	r,err:=e.Apply(p);if err!=nil{t.Fatal(err)}
	if !r.Success || r.Applied!=2 || len(o.files)!=2{t.Fatalf("%+v",r)}
	if o.calls[0]!="create:a.go" || o.calls[1]!="create:b.go"{t.Fatalf("order: %v",o.calls)}
}

func TestMultiFileEditor_ValidatesBeforeMutation(t *testing.T) {
	o:=newRecordingOps(); e,_:=NewMultiFileEditor(o)
	p:=MultiFilePlan{ID:"p1",Project:CodeProject{ID:"p",Root:t.TempDir()},Changes:[]MultiFileChange{
		{ID:"1",Operation:SourceCreate,File:sf("a.go"),Content:"x"},
		{ID:"2",Operation:SourceCreate,File:SourceFile{Path:"b.go",Language:"go",ProjectID:"other"},Content:"y"},
	}}
	if _,err:=e.Apply(p);err==nil{t.Fatal("expected validation error")}
	if len(o.files)!=0{t.Fatal("mutation occurred before validation")}
}

func TestMultiFileEditor_RollsBackOnFailure(t *testing.T) {
	o:=newRecordingOps(); o.files["a.go"]="original"
	e,_:=NewMultiFileEditor(o)
	p:=MultiFilePlan{ID:"p1",Project:CodeProject{ID:"p",Root:t.TempDir()},Changes:[]MultiFileChange{
		{ID:"1",Operation:SourceEdit,File:sf("a.go"),OldText:"original",NewText:"changed"},
		{ID:"2",Operation:SourceCreate,File:sf("fail.go"),Content:"bad"},
	}}
	o.failOn="fail.go"
	r,err:=e.Apply(p);if err!=nil{t.Fatal(err)}
	if r.Success || !r.RolledBack || r.Applied!=1{t.Fatalf("%+v",r)}
	if o.files["a.go"]!="original"{t.Fatalf("rollback failed: %q",o.files["a.go"])}
}

func TestMultiFileEditor_RejectsDuplicateIDs(t *testing.T) {
	o:=newRecordingOps(); e,_:=NewMultiFileEditor(o)
	p:=MultiFilePlan{ID:"p1",Project:CodeProject{ID:"p",Root:t.TempDir()},Changes:[]MultiFileChange{
		{ID:"same",Operation:SourceCreate,File:sf("a.go")},
		{ID:"same",Operation:SourceCreate,File:sf("b.go")},
	}}
	if _,err:=e.Apply(p);err==nil{t.Fatal("expected duplicate id rejection")}
}
