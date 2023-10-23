# capybara

a working in progress programming language developed by golang, aim to implement a "modern" language from scratch, with modern type system and popular language keyword/syntax. capybara is backed by llvm. Implementation of capybara make it a good practice of how modern language is compiled and executed.

the compilation phase include
```
  ast
[emit phase, type check, type substitute]
  -> capybara IR
[dominator frontier analysis, place PHI]
  -> capybara SSA IR
[codegen using go llvm api]
  -> llvm IR
```

capybara codebase is strongly "inspired" by gocaml. As for language feature, capybara try to absorb nutrition from all kind of modern language, rust, golang scala, koka. The importance of bootstrap a program language of capybara is a process of learning type system, modern compiler design(design a IR and map it from frontend and backend), llvm api.

language feature:

## features

- def modifiable
```
let x = 100
x = 101 // every def is modifiable
```
every modified def generate a new def in SSA form
- IF/LOOP logic
```
if a < 123 then 20 else 21; // then and else block last statement give the return value of whole if statement

for (a < 10) { a = a+2 }   // loop statement consist of a condition and a following block
```
- record, tuple
```
type person = rec{age:int};
let b = person{age:10};

type ss = tup(int);
let b = ss(121);    // tuple is regard as record with implicit key of 0, 1, 2...
```
- record method
```
type person = rec{age:int};
fun (p person) incre(): int = {  // go like method definition
    p.age + 1
};
```
- enum
```
type sport = enum{
    running,
    swimming,
    cycling
};
fun f_enum(): int = {
    let b = sport.running;
    b.discriminant  // each enum value is assigned a discriminant value. this design is borrowed from rust
}
```
- option and match
```
type some = tup[T](T);
type option = enum[P]{  // option is implemented by enum + tuple, also borrowed from rust
    none,
    some[P]
};
fun f_option(): int = {
    let b = option.some[int](121);
    let r = 0;
    match b {
    case option.some[int](a):  // here a destruction happened, `a` value from some tuple is destructed with polymorphism type
        r = a
    case _:
        r = 11
    };
    r
}
```
- trait
```
type person = rec{age:int};
type counter = trait{
    incre(): int
};

fun (p person) incre(): int = {
    p.age + 1
};
let c:counter;
c = person{age:10};  // trait variable `c` is assigned with record value
c.incre(1)
```
- parametric polymorphism
```
fun f_g[T](a:T): T = {
    a
};
let x = f_g[int](10) // this will give x with type int
```
checkout more example at `generics_xxx.txt`，capybara is currently not supporting type reconstruction. For any parametric polymorphism term, it must be supplied with type argument denoted as `[int]` or `[some_var]`. For example, `f_g[int](10)` is supplied with int as type argument and `10` as value argument.

parametric polymorphism support function, method declaration and inside record, trait as record member type and trait method parameter.

For more parametric polymorphism implementation, checkout blog https://zhuanlan.zhihu.com/p/650582139 (blog is in Chinese)
- bounded quantification
```
type person = rec{age:int};
type counter = trait{
    add(a:int): int
};

fun (p person) add(a:int): int = {
    p.age + a + 1
};

fun f_bound1[T:counter](c:T, a:int): int = {
    c.add(a)
};

let b = person{age:100};
f_bound1[person](b, 1)
```
Bounded quantification is occurred whenever parametric polymorphism is occurred. Parametric polymorphism without bound can be seen as it has a lowest type bound. For more bounded quantification implementation, checkout https://zhuanlan.zhihu.com/p/662789488 (blog is in Chinese)