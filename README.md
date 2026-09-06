# QuarkLangLibs-Cleg

**QuarkLang 官方库（cleg）——全面 GUI 框架（Qt 参考），官方认证。**

## 核心设计

- **ClegNode 核心接口**（dynamic 协议）：`render` / `setStyle(jsonText)` / `getStyle` / `getX` / `getY` / `getW` / `getH`
- **47 类组件**全部实现 ClegNode（window/标准容器/文本输入/按钮/数值状态/数据视图/菜单栏/日期时间/其它，见下）
- **全节点 Style（HashTable<String,String>）驱动渲染**，语义键采用 **QSS**（Qt Style Sheets）命名：
  ```qk
  label.style["text"] = "HELLO";
  label.style["color"] = "52,211,235";            // r,g,b
  label.style["background-color"] = "240,98,146";
  label.style["font-size"] = "46";                // px
  label.style["font-family"] = "Adwaita Sans, Liberation Sans, monospace";  // 字体回退链
  label.style["border-radius"] = "12";
  label.style["font-weight"] = "bold";            // v2
  label.style["selection-background-color"] = "52,211,235";  // 列表/表格选中
  label.style["selection-color"] = "8,28,46";
  ```
- **setStyle(string) 是真正入口**：string = **要解析的 JSON 字符串本体**（不是文件名）
  ```qk
  label.setStyle(`{"text": "HELLO", "color": "52,211,235", "font-size": "46", "font-family": "Liberation Sans, monospace"}`);
  ```
  **`@styleConfigure(file)` 给 setStyle 服务**：读 file → 文件内容字符串 → 交给 setStyle
  ```qk
  label.setStyle("") @styleConfigure(file::new("/tmp/label.json"));
  ```
- 字体回退链：`font-family` 逐名探测系统字体（Linux `/usr/share/fonts`、macOS `/System/Library/Fonts`、Windows `C:\Windows\Fonts`），FreeType 光栅（Linux）渲染，链末端内置 5×7 位图兜底——跨系统一致

## 组件全集（47 类，全部 impl ClegNode）

- **窗口/容器**：ClegMainWindow（默认窗口）、ClegDialog、ClegFrame、ClegGroupBox、ClegScrollArea、ClegSplitter、ClegDockWidget、ClegMdiArea、ClegMdiSubWindow、ClegStackedWidget、ClegTabWidget、ClegToolBox
- **文本输入/显示**：ClegLabel、ClegLineEdit、ClegTextEdit、ClegPlainTextEdit、ClegTextBrowser
- **按钮类**：ClegButton（=QPushButton）、ClegCheckBox、ClegRadioButton、ClegDialogButtonBox
- **数值/状态**：ClegProgressBar、ClegSlider、ClegScrollBar、ClegDial、ClegSpinBox、ClegDoubleSpinBox、ClegLCDNumber
- **数据视图**：ClegComboBox、ClegListWidget、ClegListView、ClegTableWidget、ClegTableView、ClegTreeWidget、ClegTreeView、ClegColumnView、ClegHeaderView
- **菜单/工具栏**：ClegMenuBar、ClegMenu、ClegToolBar、ClegStatusBar
- **日历/时间**：ClegCalendarWidget、ClegDateEdit、ClegTimeEdit
- **其它**：ClegSizeGrip、ClegOpenGLWidget、ClegVideoWidget

每类统一：`struct + impl ClegNode`（定制 render / getStyle / getX / getY / getW / getH / new）+ 状态字段 + 专有方法（`setChecked` / `setValue` / `currentIndex` / `addItem` / `setPlainText` / `display` …）+ `setStyle(jsonText)`。

## 事件接口族（组件协议：用户直接用类实现）

内置接口（dynamic，`qksignal_emit` 派发，有则调用、无则忽略）：

`ClegClickable`（onClicked/onPressed/onReleased）· `ClegCheckable`（onToggled）· `ClegEditable`（onTextChanged/onReturnPressed）· `ClegValueable`（onValueChanged）· `ClegSelectable`（onCurrentIndexChanged）· `ClegItemable`（onItemClicked）· `ClegCellable`（onCellClicked）· `ClegCloseable`（onCloseRequested）· `ClegActionable`（onTriggered）

```qk
// 组件类扩展
impl ClegButton ClegClickable { fn onClicked(self) void { ... } ... } ClegButton;
// 自定义类实现（可多接口组合）
impl MyWatcher ClegClickable { ... } MyWatcher;
impl MyWatcher ClegSelectable { fn onCurrentIndexChanged(self, idx int) void { ... } } MyWatcher;

clegsignal::emit(b, "clicked");                        // 无参
clegsignal::emitArgs(w, "onCurrentIndexChanged", 42);  // 带参（int）
clegsignal::emitText(w, "onTextChanged", "HELLO");     // 带参（String）
```

## 布局 / 信号

```qk
items List<ClegNode> = List::new();
items.append(label); items.append(btn); items.append(slider);
cleglayout::vbox(items, 40, 60, 300, 40);   // vbox / hbox / grid（Qt 布局语义：重排节点 style["pos"]）

clegsignal::emit(btn, "clicked");           // 触发节点 onClicked 方法（有则调用）
```

## 渲染后端

软件光栅帧缓冲（线性 u32、预分配、零分配热路径；4K 全屏 2.5ms）：
- 原语：`qkcleg_create/clear/rect/roundrect/text/text_ex/frame`、`qkstyle_get/num/cr`、`qksignal_emit`、`qkcleg_style_parse/load`、`qkfile_read/write`、`qkscreen_open/present/close`
- 屏幕宿主跨系统：Linux=X11（实测窗口弹出）、Windows=GDI（交叉编译产出 PE32+）、其它=PNG 帧输出
- `cleg::show(w,h,title)` / `cleg::present()` / `cleg::hide()`（`cleg::init/frame` 帧输出模式）

## Qt 对齐路线

官方研读差距表见 **`QT-QSS-to-CLEG-GAP.md`**（Qt 6.11 官方文档核对：QSS 属性全集 ~100 键、45 伪状态、36 子控件、191 类清单、信号槽模型；cleg 差距 C1–C13 必有 / D1–D19 应有 / E1–E10 可选）。

已落地：QSS 键（color/background-color/font-size/font-family/border-radius/selection-*）、字体回退链、圆角、布局（vbox/hbox/grid）、信号（emit→onXxx）、选中态、接口泛型（List<ClegNode>）。

## 认证信息

- 库名：`cleg`（`import "cleg"`）
- 语言版本：QuarkLang v0.2（接口 dynamic 派发 / `expand interface X;` 语句 / 运算符重载 / 多 impl 聚合 / 函数重载）
- 运行时版本：见主仓库 `engineVersion`；光栅基准：fill4K 2.5ms、文本 58ns/字符
- 认证方：QuarkLang 官方项目

> 官方库=认证；光栅/字体/屏幕宿主在 QuarkLang 运行时内部；窗口系统（X11/GDI）宿主已实测（Linux）/交叉编译（Windows）/帧输出（其它）。
