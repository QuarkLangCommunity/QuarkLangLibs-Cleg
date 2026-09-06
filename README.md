# QuarkLangLibs-Cleg

**QuarkLang 官方库（cleg）——轻量 GUI 框架，官方认证。**

## 设计

- **ClegNode 是核心接口**（dynamic 协议）：`render` / `getStyle` / `getX` / `getY` / `getW` / `getH`
- **默认窗口 ClegWindow 实现 ClegNode**；`ClegLabel` / `ClegButton` 同样实现
- **所有节点有 Style（HashTable<String,String>）**——style 决定渲染：
  ```qk
  label.style["text"] = "HELLO CLEG";
  label.style["pos"]  = "96,168";
  label.style["size"] = "4";
  btn.style["bg"]     = "240,98,146";
  ```
- 渲染后端 v1：CPU 光栅帧缓冲（线性 u32、预分配、零分配热路径；4K 全屏填充 2.5ms，60fps 内）；GPU/Vulkan 后端走 QuarkLangLibs-Vulkan 声明集（路线 v2）

## 使用方法

把 `cleg.qk` 放源码目录：

```qk
import "cleg";

fn main(io IOStream) {
    cleg::init(1280, 800);
    win ClegWindow = ClegWindow::new();
    win.style["title"] = "MY WINDOW";

    label ClegLabel = ClegLabel::new();
    label.style["text"] = "HELLO CLEG";
    label.style["pos"] = "96,168";

    btn ClegButton = ClegButton::new();
    btn.style["text"] = "CLICK ME";
    btn.style["pos"] = "96,248";

    // 接口动态派发：render 按实际实现调用（多 impl 聚合，方法不重叠）
    cleg::runTree(win, label, btn);
    cleg::frame("/tmp/cleg-frame.png");   // v1：输出渲染帧（真实环境为交换链提交）
}
```

## 组件

| 节点 | 实现 ClegNode | 关键 style 键 |
|---|---|---|
| `ClegWindow` | ✓（默认窗口） | `bg` / `title` / `winColor` |
| `ClegLabel` | ✓ | `text` / `color` / `size` / `pos` |
| `ClegButton` | ✓ | `text` / `bg` / `color` / `pos` / `size` |

`cleg::` 空间：`init(w,h)` / `runTree(win, label, btn)` / `frame(path)`。

## 认证信息

- 库名：`cleg`（`import "cleg"`）—— `ClegNode` 接口（dynamic） + `cleg::` 空间
- 语言版本：QuarkLang v0.2（接口动态派发 / `style["k"]=v` 索引赋值 / 多 impl 聚合 / 运算符重载）
- 运行时版本：见主仓库 `engineVersion`；光栅内核基准：fill4K 2.5ms、文本 58ns/字符
- 认证方：QuarkLang 官方项目

> 官方库=认证；光栅内核（qkcleg_*）在 QuarkLang 运行时内部；窗口系统（X11/Win32/Wayland）与 Vulkan 交换链属宿主上层（v2 路线）。
