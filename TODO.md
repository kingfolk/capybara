# TODO

basic data structure/language keyword
- [X] Fixed size array
- [X] IF/LOOP
- [X] struct
- [X] SOME/NONE
- [ ] extern keyword
- [ ] pointer

language feature
- [X] procedural paradigm and SSA IR
- [ ] exception/exception handling
- [ ] closure
- [X] trait
- [ ] subtype
- [X] rank-1 polymorphism - parametric polymorphism/generics
- [ ] parametric polymorphism expansion implementation, same as rust static dispatch
- [ ] type bound/bounded quantification
- [ ] ad-hoc polymorphism
- [ ] type reconstruction
- [ ] functional feature, like effect in Koka
- [ ] gc and memory safe(after pointer is done)

engineering
- [ ] use LLJIT instead of MCJIT
- [ ] AOT
- [X] better UT
- [ ] import and module system
- [ ] system package
- [ ] `use` keyword from rust
- [ ] command line tool


delicate work
- [ ] bootstrapping

minor
- [] type check for all ir. types are check and safe before codegen
- [] print information improved for generics type
- [] simple enum and int has implicit cast. need a cast operator to do it
- [] llvm switch instruction for enum match
