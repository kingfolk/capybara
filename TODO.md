# TODO

基本数据结构/关键词
- [X] 数组
- [X] IF/LOOP
- [X] struct
- [ ] SOME/NONE
- [ ] extern关键词
- [ ] 指针

语言特性
- [X] 过程式语法和SSA IR
- [ ] 异常
- [ ] 闭包
- [ ] trait
- [ ] subtype
- [X] rank-1 多态 - parametric polymorphism模板类型
- [ ] type bound/bounded quantification受限量化
- [ ] Ad-hoc polymorphism函数重载
- [ ] 参数重载运行时expansion实现，参考rust
- [ ] 类型重建
- [ ] 更多函数式和类型特性，例如effect，参考koka
- [ ] gc以及内存安全

工程化
- [ ] 使用LLJIT
- [ ] AOT编译
- [X] 更好的UT
- [ ] import模块系统
- [ ] 系统包
- [ ] 类似于rust的use
- [ ] 编译命令行等tool


花活
- [ ] 自举

minor
- [] 各环节的类型检查，目前很多类型报错在codegen，但需要在semantics就报出来
- [] generics的类型print优化，可以清楚看清有没有被替换等
- [] simple enum和int存在隐式转换，需要一个cast算子来合理化这样的转换
- [] llvm switch指令