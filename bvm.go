package main

import (
        "fmt"
        "os"
        "log"
        "strconv"
        //"github.com/kr/pretty"
        "github.com/llir/llvm/asm"
        "github.com/llir/llvm/ir"
        "github.com/llir/llvm/ir/constant"
        "github.com/llir/llvm/ir/types"
        "github.com/llir/llvm/ir/value"
)

func printUsage() {
        fmt.Printf("blessedvirginmary usage:\n")
        fmt.Printf("    blessedvirginmary <input>.ir [<input2>.ir [<input3>.ir ... ] ]\n")
}

func printFuncSig(f *ir.Function) {
        fmt.Printf("%s() {\n", f.Name)
}

func getConstant(c constant.Constant) string {
        switch c := c.(type) {
        case *constant.Int:
                return fmt.Sprintf("%d", c.X)
        default:
                return ""
        }
}

func getLValue(v value.Value) string {
        switch val := v.(type) {
        case value.Named:
                return "local[r" + val.GetName() + "]"
        case constant.Constant:
                return getConstant(val)
        default:
                return ""
        }
}

func getRValue(v value.Value) string {
        switch val := v.(type) {
        case value.Named:
                return "${local[r" + val.GetName() + "]}"
        case constant.Constant:
                return getConstant(val)
        default:
                return ""
        }
}

func printIcmp(inst ir.InstICmp) {
	var operand string
        switch inst.Pred {
        case ir.IntNE:
				operand = "neq"
        case ir.IntEQ:
				operand = "eq"
		case ir.IntUGT:
		case ir.IntSGT:
				operand = "gt"
		case ir.IntUGE:
		case ir.IntSGE:
				operand = "ge"
		case ir.IntULT:
		case ir.IntSLT:
				operand = "lt"
		case ir.IntULE:
		case ir.IntSLE:
				operand = "le"
		default:
				operand = "eq"
        }

		fmt.Printf("local[r%s]=`if [ \"%s\" -%s \"%s\" ]; then echo false; else echo true; fi`\n", inst.Name, getRValue(inst.X), operand, getRValue(inst.Y))
        return
}

func instAllocaHelper(inst *ir.InstAlloca) {
        switch t := inst.Typ.Elem.(type) {
        case *types.ArrayType:
                for idx := 0; idx < int(t.Len); idx++ {
                        fmt.Printf("local[s%s_%d]=0;", inst.Name, idx)
                }
                fmt.Printf("\n%s=s%s\n", getLValue(inst), inst.Name)
        default:
                fmt.Printf("local[s%s]=0\nlocal[r%s]=local[s%s]\n", inst.Name, inst.Name, inst.Name)
        }
}

func printInstruction(inst ir.Instruction) {
        switch inst := inst.(type) {
        /* Memory Instructions */

        case *ir.InstAlloca:
                instAllocaHelper(inst)
                return
        case *ir.InstLoad:
                fmt.Printf("local[r%s]=${local[%s]}\n", inst.Name, getRValue(inst.Src))
                return
        case *ir.InstStore:
                fmt.Printf("local[%s]=%s\n", getRValue(inst.Dst), getRValue(inst.Src))
                return
        case *ir.InstGetElementPtr:
                index, err := strconv.Atoi(getRValue(inst.Indices[1]))
                if err != nil {
                        panic("")
                }
                fmt.Printf("%s=%s_%d\n", getLValue(inst), getRValue(inst.Src), index)
                return

        case *ir.InstCall:
                fmt.Printf("%s local[@]\n", getLValue(inst.Callee))
                /*for _, arg := range inst.Args {
                        fmt.Printf("%s", getRValue(arg))
                }*/
                fmt.Printf("\n")

        /* Math Instructions */

        case *ir.InstAdd:
                fmt.Printf("%s=$(expr %s + %s)\n", getLValue(inst), getRValue(inst.X), getRValue(inst.Y))
                return
        case *ir.InstSub:
                fmt.Printf("%s=$(expr %s - %s)\n", getLValue(inst), getRValue(inst.X), getRValue(inst.Y))
                return
        case *ir.InstMul:
                fmt.Printf("%s=$(expr %s \\* %s)\n", getLValue(inst), getRValue(inst.X), getRValue(inst.Y))
                return
        case *ir.InstSDiv:
                fmt.Printf("%s=$(expr %s / %s)\n", getLValue(inst), getRValue(inst.X), getRValue(inst.Y))
                return
        case  *ir.InstICmp:
                printIcmp(*inst)
                return
        case *ir.InstSRem:
                fmt.Printf("%s=$(expr %s %% %s)\n", getLValue(inst), getRValue(inst.X), getRValue(inst.Y))
                return
        // What about UDiv, URem?
        // Floating Point Instructions?

        default:
                panic(fmt.Sprintf("Unknown instruction %s", inst))
        }
}

func printFuncBlock(b *ir.BasicBlock, funcname string) {
        for _, inst := range b.Insts {
                printInstruction(inst)
        }
        switch term := b.Term.(type) {
        case *ir.TermRet:
                fmt.Printf("local[ret]=%s\n", getRValue(term.X))
        case *ir.TermCondBr:
                fun1 := "_br" + term.TargetTrue.Parent.Name + term.TargetTrue.Name
                fun2 := "_br" + term.TargetFalse.Parent.Name + term.TargetFalse.Name
                fmt.Printf("if [ $r%s ]; then %s local[@]; else %s local[@]; fi\n", getLValue(term.Cond), fun1, fun2)
                fmt.Printf("local=${!?}\n")
                switch targetterm := term.Succs()[0].Term.(type) {
                case *ir.TermBr:
                        fmt.Printf("%s local[@]\n", "_br" + funcname + targetterm.Target.Name)
                }
                fmt.Printf("local=${!?}\n")
        }
}

func convertFuncToBash(f *ir.Function) {
        // Top level function
        fmt.Printf("%s() {\n", f.Name)
        fmt.Printf("local=${!1}\n")
        fmt.Printf("%s\n", "_br" + f.Name + f.Blocks[0].GetName() + " local[@]")
        fmt.Printf("local=${!?}\n")
        fmt.Printf("return local[@]\n")
        fmt.Printf("}\n")

        // Blocks
        for _, block := range f.Blocks {
                fmt.Printf("%s() {\n", "_br" + f.GetName() + block.Name)
                fmt.Printf("local=${!1}\n")
                printFuncBlock(block, f.GetName())
                fmt.Printf("return local[@]\n")
                fmt.Printf("}\n")
        }
}

func main() {
        args := os.Args[1:] // Slice the args after the program name
        if len(args) == 0 {
                printUsage()
                os.Exit(1)
        }

        for i := 0; i < len(args); i++ {
                parsedAsm, err := asm.ParseFile(args[i])
                if err != nil {
                        log.Fatal(err)
                }
                //pretty.Println(parsedAsm)
                for _, f := range parsedAsm.Funcs {
                        convertFuncToBash(f)
                }
        }

        fmt.Println("declare -A local")
        fmt.Println("main local[@]")
        fmt.Println("exit ${local[ret]}")
}

