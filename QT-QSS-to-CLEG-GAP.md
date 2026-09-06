# Qt Widgets 研读笔记 —— QSS/Qt → cleg 对照差距表

- 目标：自研 GUI 框架 **cleg**（`ClegNode` 接口 + `style HashTable<String,String>` + 47 组件 + 字体回退链）全面对齐 Qt Widgets 使用方式。
- 事实核对方式：全部内容摘自 Qt 6.11 官方文档（doc.qt.io），每节标注来源 URL；关键差异点（如 QSlider 无 `::sub-page`、QLabel 不支持 `:hover`、QSS 默认不继承 font/color）均按官方参考原文标注。
- 本文所有 "cleg 现状" 均基于实读代码：`cleg.qk`（47 个 struct + impl ClegNode）、`internal/lang/clegfb.go`（CPU 帧缓冲 + 字体回退链）。

---

## 1. QWidget 基础属性（表：属性名 | 作用 | cleg style 键建议）

QWidget 是 Qt Widgets 所有 UI 对象的基类：每个 widget 都是矩形、接收鼠标/键盘/触摸事件、自身绘制、可裁切、按 Z 序排列；无父 widget 即为顶层窗口（window）。（来源：<https://doc.qt.io/qt-6/qwidget.html>、<https://doc.qt.io/qt-6/qwidget.html#details>）

**补充机制说明（来源：<https://doc.qt.io/qt-6/stylesheet-syntax.html#setting-qobject-properties>）**
- Qt 属性系统：类内 `Q_PROPERTY` 声明 + `setProperty/Property` 访问；**任何 designable Q_PROPERTY 都可通过 QSS `qproperty-<name>` 语法设置**（如 `QPushButton { qproperty-iconSize: 20px 20px; }`），且 QSS **属性选择器** `QPushButton[flat="false"]` 可按任意属性（含动态属性）匹配，`~=` 匹配 QStringList 属性。
- QWidget 本身**没有** `geometryChanged` 信号（Qt Quick 的 QQuickItem 才有）；尺寸变化走 `resizeEvent()`/`moveEvent()` 事件，另有三条自有信号：`customContextMenuRequested(QPoint)`、`windowTitleChanged(QString)`、`windowIconChanged(QIcon)`。

| Qt 属性（property） | 类型 | 作用 | cleg style 键建议 |
|---|---|---|---|
| `geometry` | QRect | 位置+尺寸（等价 x/y/w/h 四合体），`setGeometry(x,y,w,h)` | `pos`（已有，`"x,y"`）+ 建议新增 `"geometry"="x,y,w,h"`（read/write） |
| `pos` / `x` / `y` | QPoint/int | 相对父 widget 的位置 | `pos`（已有） |
| `size` / `width` / `height` | QSize/int | 尺寸 | 已有 `w`/`h` 字段；建议只读键 `size` |
| `sizeHint()` | QSize（只读虚函数） | **推荐尺寸**：布局用于计算理想大小；自定义 widget 应重写 `sizeHint()` + `setSizePolicy()` | 建议新增 `size-hint`（或方法 `getPreferredW/H`） —— cleg 现状无 |
| `minimumSize` / `minimumWidth` / `minimumHeight` | QSize/int | 最小尺寸（布局不可压破） | 建议 `min-width`/`min-height`（QSS 同名键！） |
| `maximumSize` / `maximumWidth` / `maximumHeight` | QSize/int | 最大尺寸 | 建议 `max-width`/`max-height` |
| `minimumSizeHint()` | QSize（只读虚函数） | 内容天然最小尺寸（QLabel 等按文本计算） | 建议 `min-content` 派生；cleg 现状无 |
| `sizePolicy` | QSizePolicy | 横/纵两轴的"伸缩策略"（见第 6 节），布局核心 | 建议 `size-policy`（值 `fixed,minimum,maximum,preferred,expanding,ignored`） |
| `enabled` | bool | 是否可交互；子 widget 继承父的禁用，`:disabled` 伪状态由此派生 | 建议 `enabled`（"false" 时走 disabled 渲染分支） |
| `visible` | bool（read/write） | 是否可见；`show()/hide()`/槽 | 建议 `visible`（"false"=skip render） |
| `toolTip` / `toolTipDuration` | QString / int | 悬停提示文本 / 显示时长 | 建议 `tooltip` |
| `statusTip` | QString | 状态栏提示（悬停时） | 建议 `status-tip` |
| `whatsThis` | QString | F1/问号上下文帮助文本 | 建议 `whats-this` |
| `windowTitle` | QString | 窗口标题（顶层窗口标题栏） | 建议 `title`（已有 ClegMainWindow/Dialog 用 `title`，Label 用 `text` —— 需区分！） |
| `windowIcon` / `windowIconChanged` | QIcon / signal | 窗口图标 | 建议 `icon` |
| `font` | QFont | 字体（setFont 会**冒泡到全部子 widget**） | 已有 `font`；建议补充 `font-size`/`font-style`/`font-weight`（QSS 键已有 font-size/font-family 部分支持） |
| `palette` | QPalette | 各颜色角色（Window/WindowText/Button/Text/Active/Disabled…） | 建议 `palette.window`/`palette.text`/`palette.button`/`palette.highlight` 等展开键；现为 `bg`/`color` 两键 |
| `cursor` | QCursor | 鼠标形状 | 建议 `cursor`（"arrow,hand,text,cross,move"） |
| `focusPolicy` | Qt::FocusPolicy | 键盘焦点策略（Never/TabFocus/ClickFocus/StrongFocus/WheelFocus） | 建议 `focus-policy` |
| `focus` (read) / `hasFocus()` | bool（只读） | 是否持有键盘焦点；`:focus` 伪状态来源 | 建议状态派生 `focus` |
| `layout` | QLayout* | 挂到 widget 上的布局；`setLayout()` | 建议 `layout`（"vbox,hbox,grid,form"）+ `cleglayout` 已有 vbox/hbox/grid 函数 |
| `layoutDirection` | Qt::LayoutDirection | LTR/RTL（布局与文本方向） | 建议 `layout-direction`（"ltr","rtl"） |
| `styleSheet` | QString | 本 widget 的 QSS 字符串（自身优先于祖先级联） | `setStyle(jsonText)` 已有 → 建议扩展为真正选择器语法 |
| `autoFillBackground` | bool | 用 palette 背景角色自动填充 | 建议 `auto-fill-background` |
| `acceptDrops` | bool | 是否接受拖放 | 建议 `accept-drops`（v2） |
| `contextMenuPolicy` | Qt::ContextMenuPolicy | 右键菜单策略（`customContextMenuRequested` 信号前提） | 建议 `context-menu-policy` |
| `cursor` 相关 `mouseTracking` | bool | 无按键悬停也收 mouseMoveEvent（hover 伪状态依赖） | 建议事件层 `mouse-tracking` |
| `inputMethodHints` | Qt::InputMethodHints | 输入法提示 | 建议 `input-method-hints`（v2） |
| `locale` | QLocale | 本地化（Qt6 属性） | 建议 `locale`（v2） |
| `accessibleName` / `accessibleDescription` | QString | 无障碍名称/描述（Qt 6.9 加 accessibleIdentifier） | 建议 `accessible-name`（v2） |
| `windowFlags` / `windowModality` / `windowOpacity` / `windowFilePath` | 组合 | 窗口标志/模态/透明度/关联文件 | 建议 `window-flags`/`window-modality`/`window-opacity`（v2） |
| `updatesEnabled` / `tabletTracking` | bool | 重绘开关 / 数位板跟踪 | 均可选 |

**信号与事件骨架（来源：qwidget.html#signals、qwidget.html#detail 事件列表）**：`customContextMenuRequested(const QPoint&)`；受保护事件虚函数：`paintEvent/resizeEvent/moveEvent/showEvent/hideEvent/closeEvent/enterEvent/leaveEvent/mousePressEvent/mouseReleaseEvent/mouseDoubleClickEvent/mouseMoveEvent/wheelEvent/keyPressEvent/keyReleaseEvent/focusInEvent/focusOutEvent/contextMenuEvent` —— 这就是 "everything is an event" 的 Qt 模型，cleg 目前完全没有事件虚函数层（只有 `render`）。

---

## 2. QSS 属性全集（表：QSS 属性 | 可用控件/全局 | 示例值 | cleg style 键建议）

官方"List of Properties"全部条目（约 100 个，含四条边展开键）均已核对，来源：<https://doc.qt.io/qt-6/stylesheet-reference.html#list-of-properties>；可样式化的 17 类控件清单：<https://doc.qt.io/qt-6/stylesheet-reference.html#list-of-stylable-widgets>。

图例：`*` 表示该属性在官方表中带星号（仅特定控件/部分版本生效）。**cleg 现状**：仅支持 `color`/`background-color`/`font-size`/`font-family`/`border-radius`（5 个 QSS 键）+ 11 个旧键（`text,items,tabs,rows,menus,size,pos,font,color,bg,title`）。

| QSS 属性 | 类别 | 可用控件/全局 | 示例值 | cleg style 键建议 |
|---|---|---|---|---|
| `margin` 及其 `-top/-right/-bottom/-left` | Box Model | 全局 box 模型 | `margin: 8px 16px` | **新增** `margin`/`margin-{top,right,bottom,left}`（现无） |
| `padding` 及其四边 | Box Model | 全局 box 模型 | `padding: 6px 12px` | **新增** `padding`/`padding-*`（现无） |
| `border` | Box Model | QLineEdit/QFrame/QPushButton/… | `border: 1px solid white` | **新增** `border`（简写合成三键） |
| `border-width` / `border-style` / `border-color` 及同位 `-top-*` 等（共 12 键） | Box Model | 同上 | `border-width: 1px; border-style: solid; border-color: #808080` | **新增** `border-{width,style,color}` |
| `border-radius` 及四角 (4 键) | Box Model | 同上（**不适用**于 QMenu*？——注意官方列表：QMenuBar/QMenu/QToolTip 无 radius！） | `border-radius: 4px` | **已有**（cleg ClegButton 支持） |
| `border-image` | Box Model（九宫格） | QFrame/QLabel/QPushButton 等 | `border-image: url(bg.png) 9` | 可选 v2 |
| `background` | Background | 全局通用（含 plain QWidget） | `background: yellow` | **新增**（`bg` 的 QSS 化别名） |
| `background-color` | Background | 全局通用 | `background-color: rgb(255,0,0)` | **已有**（3 通道解析） |
| `background-image` | Background | 全局通用 | `background-image: url(bg.png)` | **新增 v2**（URL 无 png 加载层） |
| `background-repeat` | Background | 全局通用 | `background-repeat: repeat-x` | 可选 v2 |
| `background-position` | Background | 全局通用 | `background-position: bottom left` | 可选 v2 |
| `background-attachment` | Background | QAbstractScrollArea 系（fixed/scroll） | `background-attachment: fixed` | 可选 v2 |
| `background-clip` | Background | 全局通用（border/padding/content 矩形） | `background-clip: padding` | **应新增 v2**（盒模型三矩形概念） |
| `background-origin` | Background | 全局通用 | `background-origin: content` | 可选 v2 |
| `alternate-background-color` | Background | QAbstractItemView | `alternate-background-color: #f0f0f0` | **应新增**（表格行交替色） |
| `accent-color` | Background | 通用（Qt6 新增） | `accent-color: blue` | 可选 |
| `font` | Font | 全局（尊重 font 属性的 widget） | `font: bold italic 12px "Times"` | 建议简写解析 |
| `font-family` | Font | 全局 | `font-family: "New Century Schoolbook"` | **已有**（走回退链） |
| `font-size` | Font | 全局（**仅 pt/px 两单位**，实例：`font-size: 12px`） | `font-size: 10pt` | **已有**（px 语义近似：scale=px/8） |
| `font-style` | Font | 全局 | `font-style: italic` | **应新增**（cleg 只有 5x7 与 TTF，无斜体合成） |
| `font-weight` | Font | 全局 | `font-weight: bold` | **应新增** |
| `letter-spacing` | Font | 全局 | `letter-spacing: 2px` | 可选 |
| `word-spacing` | Font | 全局 | `word-spacing: 3px` | 可选 |
| `color` | Text | 全局（尊重 palette 的 widget） | `color: red` | **已有** |
| `text-align` | Text | QProgressBar 文本等 | `text-align: center` | **应新增**（文本无对齐） |
| `text-decoration` | Text | 全局 | `text-decoration: underline` | **应新增**（下划线/删除线/上划线） |
| `selection-color` / `selection-background-color` * | Text | QLineEdit/QListView/QTableView | `selection-background-color: #0078d4` | **应新增**（列表选中态） |
| `placeholder-text-color` * | Text | QLineEdit/QComboBox | `placeholder-text-color: gray` | **应新增** |
| `lineedit-password-character` / `lineedit-password-mask-delay` * | Text | QLineEdit | `lineedit-password-character: 9679` | 可选 |
| `gridline-color` * | 表 | QTableView | `gridline-color: gray` | **应新增**（表线色） |
| `paint-alternating-row-colors-for-empty-area` * | 表 | QTableView | `paint-alternating-row-colors-for-empty-area: 1` | 可选 |
| `width` / `height` | 尺寸 | **子控件**（对 widget 基本无效，见官方 Warning：固定 widget 尺寸请用 min/max 相同值） | `QSpinBox::down-button { height: 10px }` | **应新增**（子控件几何） |
| `min-width` / `min-height` / `max-width` / `max-height` | 尺寸 | 子控件 & widget | `min-width: 40px` | **应新增**（正是第 1 节 QWidget min/max 的 QSS 等价） |
| `image` * / `image-position` | 子控件图片 | 子控件（`image` 设置隐式给宽高） | `QComboBox::drop-down { image: url(a.png) }` | 可选 v2 |
| `icon` / `icon-size` | 图标 | `icon` 仅 QPushButton（≥5.15）；`icon-size` 多控件 | `icon-size: 20px 20px` | **应新增**（按钮图标） |
| `top` / `right` / `bottom` / `left` | 子控件定位 | 子控件（relative/absolute 两种方案） | `QSpinBox::down-button { bottom: 2px }` | 可选 v2 |
| `position` | 子控件定位 | relative（默认）/absolute | `position: relative` | 可选 v2 |
| `subcontrol-origin` / `subcontrol-position` | 子控件定位 | 子控件 | `subcontrol-origin: margin` | 可选 v2 |
| `spacing` * | 布局 | QCheckBox/QRadioButton/QGroupBox/QMenuBar | `spacing: 8px` | **应新增**（cleglayout 已有 spacing 参数 → 提升为 style 键） |
| `button-layout` | 框 | QDialogButtonBox / QMessageBox（0=Win 1=Mac 2=KDE 3=GNOME 5=Android） | `* { button-layout: 2 }` | **应新增**（按钮排列规则） |
| `dialogbuttonbox-buttons-have-icons` | 框 | QDialogButtonBox | `1` | 可选 |
| `messagebox-text-interaction-flags` * | 框 | QMessageBox | `Qt::TextBrowserInteraction` | 可选 |
| `titlebar-show-tooltips-on-buttons` | 窗 | 顶层窗口按钮 | `1` | 可选 |
| `widget-animation-duration` * | 动画 | 全局 | `150` | 可选 |
| `show-decoration-selected` * | 视图 | QListView | `0` | 可选 |
| `opacity` * | 效果 | 全局 | `0.5` | 可选 |
| `outline` / `outline-color/style/offset` 及四角 radius（7 键） | 焦点框 | 全局 | `outline: none` | **应新增**（焦点高亮样式，Qt3 遗留但经典） |
| `-qt-background-role` / `-qt-style-features` | 内部 | 私有 | — | 忽略 |

**复杂控件专属（官方"List of Stylable Widgets"逐条核对，来源同参考页）**

| 控件 | 可样式部分（子控件/属性） | cleg 建议键 |
|---|---|---|
| QPushButton | box model；`:default :flat :checked :open :closed`；`::menu-indicator`；`icon`（5.15+）。**坑：只设 background-color 可能不生效，除非同时设 border（原生 border 覆盖）** | `menu-indicator` 可选；记录该坑 |
| QProgressBar | box model；`::chunk`（Contents 矩形内）；`text-align` 定位文字；`:indeterminate` | **新增 `chunk`**（进度块颜色/宽度） |
| QSlider | box model；`::groove`（Contents 内）+ `::handle`（groove Contents 内移动）。**注意：官方**没有 `::sub-page`/`::add-page`（那是 QScrollBar 的子控件）——常见误解请纠正 | 新增 `groove`/`handle`/`tick` |
| QScrollBar | `::handle` `::add-line` `::sub-line` `::sub-page` `::add-page` `::up-arrow/down-arrow/left-arrow/right-arrow`；`:horizontal`/`:vertical` | 新增 `handle`/`sub-page`/`add-page` |
| QTabBar | `::tab`（支持 `:first :last :middle :only-one :selected :next-selected :previous-selected`）、`::close-button`、`::tear`、`::scroller`、`QTabBar QToolButton`；`:top/left/right/bottom`；`alignment` 属性 | 新增 `tab`/`tab-selected` |
| QTabWidget | `::pane` `::left-corner` `::right-corner` `::tab-bar`（subcontrol-position 调 tab 位置） | 新增 `pane` |
| QHeaderView | `::section`（支持 `:first :last :middle :only-one :selected :checked :next-selected :previous-selected`）、`::up-arrow`/`::down-arrow`（排序指示） | 新增 `section` |
| QToolTip | box model（padding 生效） | 新增全局 `tooltip` 主题键 |
| QMenu | box model；`::item`（`:selected :default :exclusive :non-exclusive`）、`::indicator`、`::separator`、`::right-arrow/left-arrow`、`::scroller`、`::tearoff` | 新增 `menu-item`/`menu-separator` |
| QMenuBar | box model；`spacing`；`::item` | 新增 `menubar-item` |
| QComboBox | box model；`::drop-down`（默认在 Padding 矩形右上）、`::down-arrow`、`placeholder-text-color` | 新增 `drop-down`/`down-arrow` |
| QSpinBox/QDoubleSpinBox/QDateEdit/QDateTimeEdit | box model（frame）；`::up-button`/`::up-arrow`/`::down-button`/`::down-arrow`；`:no-frame` | 新增 `up-button`/`down-button` |
| QCheckBox / QRadioButton | box model；`::indicator`（Contents 左上）；`spacing` 文字与指示器间距 | 新增 `indicator`/`indicator-checked` |
| QGroupBox | box model；`::title`（按 textAlignment 定位）；checkable 时 `::indicator`+`spacing` | 新增 `title` |
| QDialog | **仅支持 `background`/`background-clip`/`background-origin`**（别的属性不生效） | 记录限制 |
| QStatusBar | **仅支持 `background`**；`::item` | 记录限制 |
| QDockWidget | `::title` `::close-button` `::float-button`；`:closable :floatable :movable :vertical`；`::separator`（在 QMainWindow 上） | 可选 |
| QDockWidget 注意事项：脱离 docking 后为原生顶层窗口，QSS 无效 | — | 记录 |
| QListView/QListWidget/QTableView/QTableWidget/QTreeView/QTreeWidget | box model；`alternate-background-color`、`selection-color`、`selection-background-color`、`show-decoration-selected`；`::item` 细粒度；scrollable 背景 `background-attachment` | 新增 `item`/`selection-bg`/`alternate-bg` |
| QSizeGrip | `width`/`height`/`image` | 可选 |
| QMainWindow | `::separator`（dock widget 分隔） | 可选 |
| QSplitter | box model；`::handle` | 新增 `handle`（分离条） |
| QColumnView | `image`（grip）、`::left-arrow`/`::right-arrow` | 可选 |
| QAbstractScrollArea（QTextEdit 等） | box model + 可滚动背景 | 可选 |
| QDialogButtonBox | `button-layout` | 新增（布局规则） |
| QMessageBox | `messagebox-text-interaction-flags` | 可选 |
| QFrame / QLabel | box model；**QLabel 不支持 `:hover`**；设置 stylesheet 后 frameStyle 自动变 StyledPanel | 记录：ClegLabel 不要做 hover |

---

## 3. 伪状态与选择器（表）

### 3.1 伪状态全集（官方 45 个，来源：<https://doc.qt.io/qt-6/stylesheet-reference.html#list-of-pseudo-states>）

| 伪状态 | 触发时机（官方描述摘要） | 适用控件举例 | cleg 建议（状态键） |
|---|---|---|---|
| `:active` | widget 位于活动窗口 | 全局 | `state=active` |
| `:alternate` | 交替行绘制（alternatingRowColors=true） | QAbstractItemView | `state=alternate` |
| `:bottom/:top/:left/:right` | 位于底/顶/左/右（tab 方位等） | QTabBar/QTabWidget | `tab-pos` 键 |
| `:checked` | 已勾选（QAbstractButton::checked） | QCheckBox/QRadioButton/QPushButton/QGroupBox | `state=checked`（字段已有 checked） |
| `:closable/:floatable/:movable` | Dock 特性位开启 | QDockWidget | 可选 |
| `:closed/:open` | 折叠/展开；QComboBox 或带菜单按钮弹出 | QTreeView/QComboBox/QPushButton | `state=open/closed` |
| `:default` | 默认按钮/默认菜单项 | QPushButton/QMenu | `state=default` |
| `:disabled` | `enabled=false` | 全局 | `state=disabled` |
| `:editable` | QComboBox 可编辑 | QComboBox | `state=editable` |
| `:edit-focus` | 编辑焦点（Qt Extended 专用） | — | 忽略 |
| `:enabled` | 与 disabled 相反 | 全局 | `state=enabled` |
| `:exclusive/:non-exclusive` | 是否处于互斥组 | QMenu/QActionGroup | 可选 |
| `:first/:last/:middle/:only-one` | 列表首位/末位/中间/唯一 | QTabBar/QHeaderView | `tab-pos=first/last/...` |
| `:flat` | flat 按钮 | QPushButton | `state=flat` |
| `:focus` | 持有输入焦点 | 全局 | `state=focus` |
| `:has-children/:has-siblings` | 树中有子/有兄弟 | QTreeView | 可选 |
| `:horizontal/:vertical` | 横向/纵向 | QScrollBar/QSlider | `orientation` 键 |
| `:hover` | 鼠标悬停 | 全局（**QLabel 除外**） | **新增 `state=hover`** |
| `:indeterminate` | 半选（部分勾选） | QCheckBox/QRadioButton | `state=indeterminate` |
| `:maximized/:minimized` | 最大/最小化 | QMdiSubWindow | `state=max/min` |
| `:movable` | 可移动 | QDockWidget | 可选 |
| `:no-frame` | 无边框 | QSpinBox/QLineEdit | `frame=none` |
| `:off/:on` | 可开关项处于关/开 | 开关类 | `state=on/off` |
| `:next-selected/:previous-selected` | 相邻项被选中 | QTabBar/QHeaderView | 可选 |
| `:pressed` | 鼠标按下 | 全局按钮 | **新增 `state=pressed`** |
| `:read-only` | 只读/不可编辑 | QLineEdit/QComboBox | `state=read-only` |
| `:selected` | 被选中（tab/菜单项/表项） | QTabBar/QMenu/QAbstractItemView | `state=selected` |
| `:unchecked` | 未勾选 | 勾选类 | `state=unchecked` |
| `:window` | 顶层 widget | 全局 | `window=true` |

### 3.2 选择器语法（来源：<https://doc.qt.io/qt-6/stylesheet-syntax.html>）

| 选择器 | 示例 | 语义 |
|---|---|---|
| 通用 | `*` | 所有 widget |
| 类型 | `QPushButton` | 该类**及其子类**（按 className()） |
| 类 | `.QPushButton` | **仅**该类（等价 `*[class~="QPushButton"]`） |
| ID | `QPushButton#okButton` | objectName 匹配（`#` 对应 `setObjectName`） |
| 属性 | `QPushButton[flat="false"]` | 任意 Q_PROPERTY（含动态属性）；`~=` 用于 QStringList 包含；`class` 是特殊属性 |
| 后代 | `QDialog QPushButton` | 任意深度后代 |
| 子 | `QDialog > QPushButton` | 直接子 widget |
| 子控件 | `QComboBox::drop-down` | `::name`，36 个子控件 |
| 伪状态 | `QCheckBox:hover:checked` | 多个伪状态=AND；`!` 取反（`:!pressed`）；`,` = OR |
| 命名空间 | `ns--MyPushButton` | 命名空间内类用 `--` 代替 `::` |
| qproperty | `QLabel { qproperty-pixmap: url(pixmap.png); }` | 设置任意 designable Q_PROPERTY（polish 时一次性生效，伪状态下无效） |
| 特异性 | CSS2 a-b-c | ID>属性/伪类>类型；**子控件不计入**；同特异性按后出现者优先 |
| 级联 | app→父→自 | **自身 stylesheet 永远优先**；不支持 `!important` |
| 继承 | — | QSS 的 font/color **默认不自动继承**（`AA_UseStyleSheetPropagationInWidgetStyles` 可开启）；而 setFont/setPalette 会向子传播 |

---

## 4. 组件清单（表：Qt 类 | 用途 | 常用属性/方法 | cleg 组件建议）

类清单与模块来源：<https://doc.qt.io/qt-6/qtwidgets-module.html>（Qt Widgets C++ Classes，共 191 个类）。QVideoWidget 属 **QtMultimedia**（<https://doc.qt.io/qt-6/qvideowidget.html>），QOpenGLWidget 在 Qt6 属 **QtOpenGLWidgets** 模块（<https://doc.qt.io/qt-6/qtopenglwidgets-module.html>），二者不是 QtWidgets 模块类。

| Qt 类 | 用途 | 常用属性/方法（已核对官方页） | cleg 组件建议 |
|---|---|---|---|
| QWidget | 一切 UI 的基类；事件+绘制+属性 | 见第 1 节 | ClegNode 接口（已有）；补事件层 |
| QFrame | 带边框/形状的矩形（基类） | frameShape/frameShadow/lineWidth（样式表自动改 frameStyle） | ClegFrame（已有） |
| QDialog | 对话框窗口基类 | exec()/open()/result()/accept()/reject()/done() | ClegDialog（已有） |
| QMainWindow | 主窗口骨架 | setCentralWidget/menuBar()/statusBar()/addToolBar/addDockWidget/restoreState | ClegMainWindow（已有，需补槽位） |
| QGroupBox | 分组框（可选勾选+标题） | setTitle/title/setCheckable/setChecked/setAlignment；信号 clicked/toggled | ClegGroupBox（已有） |
| QSplitter | 可拖分隔条容器 | setOrientation/setSizes/setStretchFactor/setHandleWidth/children() | ClegSplitter（已有） |
| QScrollArea | 滚动视口 | setWidget/setWidgetResizable/widget()/setAlignment | ClegScrollArea（已有） |
| QTabWidget | 多标签页 | addTab/insertTab/removeTab/setCurrentIndex/currentChanged(int)/tabBar()/setTabText | ClegTabWidget（已有；无 tabBar 子对象） |
| QStackedWidget | 单页切换栈 | addWidget/setCurrentIndex/currentChanged(int)/widgetAdded/removed | ClegStackedWidget（已有） |
| QToolBox | 工具箱（折叠列） | addItem/setCurrentIndex/currentChanged/setItemText/setItemEnabled | ClegToolBox（已有） |
| QMdiArea | MDI 多文档区 | addSubWindow/activeSubWindow/setActiveSubWindow/cascadeSubWindows/tileSubWindows/closeActiveSubWindow | ClegMdiArea（已有） |
| QMdiSubWindow | MDI 子窗口 | setWidget/widget/setWindowTitle/setWindowFlags；:maximized 伪态 | ClegMdiSubWindow（已有） |
| QDockWidget | 可停靠面板 | setWidget/setWindowTitle/setFeatures(DockWidgetFeature)/setAllowedAreas；title 子控件 | ClegDockWidget（已有） |
| QDialogButtonBox | 标准按钮排（OK/Cancel…） | addButton/standardButtons/button()/accepted/rejected/clicked(QAbstractButton*) | ClegDialogButtonBox（已有；无标准按钮枚举） |
| QFormLayout | 表单布局（标签+字段对） | addRow(label, field)/setLabelAlignment/setSpacing/setFieldGrowthPolicy；`TakeRowResult` | **缺** → ClegFormLayout 方法 |
| QGridLayout | 网格布局 | addWidget(w,row,col[,rowSpan,colSpan])/setSpacing/setColumnMinimumWidth/setRowStretch/setColumnStretch/setOriginCorner | **缺**（cleglayout::grid 仅等宽网格） |
| QHBoxLayout / QVBoxLayout / QBoxLayout | 盒式布局 | addWidget/addLayout/addSpacing/addStretch/addSpacerItem/setSpacing/setStretch/setDirection/insertWidget | **缺** → ClegLayout 兼容层 |
| QSpacerItem | 弹性占位 | width/height/expandX/expandY/sizeHint | **缺** → ClegSpacer |
| QLabel | 文本/富文本/图片 | setText/setPixmap/setWordWrap/setAlignment/setOpenExternalLinks/setTextFormat | ClegLabel（已有；**勿加 hover**） |
| QLineEdit | 单行输入 | setText/text/setPlaceholderText/setEchoMode/setMaxLength/setValidator/clear/selectAll；textChanged/textEdited/returnPressed/editingFinished/cursorPositionChanged | ClegLineEdit（已有；无占位符/密码模式） |
| QTextEdit | 富文本编辑（QAbstractScrollArea） | setPlainText/setHtml/toPlainText/insertPlainText/append/setReadOnly/selectAll；textChanged/cursorPositionChanged | ClegTextEdit（已有） |
| QPlainTextEdit | 纯文本编辑（性能型） | setPlainText/toPlainText/appendPlainText/blockCount/setMaximumBlockCount/setLineWrapMode | ClegPlainTextEdit（已有） |
| QTextBrowser | 只读富文本浏览 | setSource/setHtml/setOpenLinks/setSearchPaths/anchorClicked/backward/forward/home/end | ClegTextBrowser（已有） |
| QStatusBar | 状态栏 | addWidget/addPermanentWidget/removeWidget/showMessage/clearMessage/currentMessage | ClegStatusBar（已有） |
| QMenuBar | 菜单栏 | addMenu/addAction/clear；::item 子控件 | ClegMenuBar（已有；无 QAction 概念） |
| QMenu | 弹出菜单 | addAction/addMenu/addSeparator/exec/popup/actions/clear；triggered(QAction*)/hovered/aboutToShow/aboutToHide | ClegMenu（已有；无动作对象） |
| QToolBar | 工具栏 | addAction/addWidget/addSeparator/setMovable/setAllowedAreas/setOrientation/setToolButtonStyle | ClegToolBar（已有） |
| QToolButton | 工具按钮（图标方块） | setIcon/setToolButtonStyle/setDefaultAction/setMenu/setPopupMode/setAutoRaise；triggered | **缺** → ClegToolButton（或并入 ClegButton 风格） |
| QPushButton | 普通按钮 | setText/setIcon/setCheckable/setAutoDefault/setDefault/setMenu；clicked(bool=false)/pressed/released/toggled | ClegButton（已有，对应 QPushButton） |
| QCommandLinkButton | Vista 命令链接按钮 | setDescription/description/setIcon/text（继承 QPushButton） | **缺** → ClegCommandLinkButton |
| QRadioButton | 单选 | setText/setChecked/setAutoExclusive；toggled/clicked | ClegRadioButton（已有；`checked` 字段已有） |
| QCheckBox | 复选 | setText/setTristate/setCheckState/setChecked；stateChanged(Qt::CheckState)/toggled | ClegCheckBox（已有） |
| QButtonGroup | 互斥组 | addButton(button,id)/removeButton/setExclusive/checkedButton/checkedId/idClicked(id) | **缺** → ClegButtonGroup |
| QSpinBox | 整数微调 | setRange/setValue/setPrefix/setSuffix/setSingleStep/setGroupSeparatorShown；valueChanged(int)/textChanged | ClegSpinBox（已有） |
| QDoubleSpinBox | 浮点微调 | 同 QSpinBox + setDecimals/setMinimum/setMaximum | ClegDoubleSpinBox（已有） |
| QDateEdit / QTimeEdit / QDateTimeEdit | 日期/时间编辑 | setDate/setTime/setDateTime/setDisplayFormat/setCalendarPopup/setMinimumDateTime；dateChanged/timeChanged/dateTimeChanged | ClegDateEdit/ClegTimeEdit（已有）；**缺 QDateTimeEdit** → ClegDateTimeEdit |
| QCalendarWidget | 日历 | setSelectedDate/selectedDate/setDateRange/setFirstDayOfWeek/setVerticalHeaderFormat/setGridVisible；selectionChanged/currentPageChanged(int,int)/clicked(QDate) | ClegCalendarWidget（已有） |
| QProgressBar | 进度条 | setRange/setValue/value/reset/setOrientation/setFormat/setTextVisible；valueChanged(int)；::chunk | ClegProgressBar（已有） |
| QSlider | 滑杆 | setRange/setValue/setOrientation/setTickPosition/setSingleStep/setPageStep/setTracking；valueChanged/sliderMoved/sliderPressed/sliderReleased | ClegSlider（已有） |
| QScrollBar | 滚动条 | 同 QAbstractSlider（valueChanged(int)/rangeChanged(int,int)/actionTriggered(int)）；Orientation | ClegScrollBar（已有） |
| QDial | 旋钮 | setRange/setValue/setNotchesVisible/setWrapping/setTracking；valueChanged | ClegDial（已有） |
| QListWidget | 列表（项驱动） | addItem/addItems/insertItem/takeItem/currentItem/currentRow/setCurrentRow/item(i)/count/clear/setSelectionMode；itemClicked/itemDoubleClicked/itemActivated/currentRowChanged/currentItemChanged/itemSelectionChanged | ClegListWidget（已有） |
| QListView | 列表（model/view） | 同 QAbstractItemView（setModel/setModelColumn/currentIndex/setCurrentIndex/clicked(QModelIndex)/doubleClicked/selectionChanged） | ClegListView（已有） |
| QComboBox | 下拉框 | addItem/addItems/insertItem/currentIndex/setCurrentIndex/currentText/setEditable/count/clear/setMaxVisibleItems；currentIndexChanged(int)/currentTextChanged(QString)/activated(int)/highlighted(int)/editTextChanged(QString) | ClegComboBox（已有） |
| QTreeWidget | 树（项驱动） | setHeaderLabels/addTopLevelItem/topLevelItem/currentItem/setCurrentItem/expandAll/setColumnCount/takeTopLevelItem；itemClicked/itemExpanded/itemCollapsed/itemDoubleClicked/currentItemChanged | ClegTreeWidget（已有） |
| QTreeView | 树（model/view） | 同 QAbstractItemView + expand/collapse/setExpanded/isExpanded | ClegTreeView（已有） |
| QTableWidget | 表格（项驱动） | setRowCount/setColumnCount/setItem/item/currentRow/currentColumn/clear/setHorizontalHeaderLabels；cellClicked(int,int)/cellDoubleClicked/cellChanged(int,int)/currentCellChanged/currentItemChanged | ClegTableWidget（已有） |
| QTableView | 表格（model/view） | 同 QAbstractItemView + setModel/setSpan/setSortingEnabled；gridline-color QSS | ClegTableView（已有） |
| QColumnView | 级联列视图 | setModel/setRootIndex/setColumnWidths/setResizeGripsVisible/updatePreviewWidget | ClegColumnView（已有） |
| QHeaderView | 表头 | setModel/currentIndex/count/resizeSection/setSortIndicator/setStretchLastSection/setDefaultSectionSize；sectionClicked(int)/sectionResized/sortIndicatorChanged(int,Qt::SortOrder) | ClegHeaderView（已有） |
| QItemDelegate | 编辑器委托抽象 | paint/sizeHint/createEditor/setEditorData/setModelData/updateEditorGeometry（继承 QAbstractItemDelegate） | **缺** → ClegItemDelegate（v2） |
| QModelIndex | 模型索引值类型 | row()/column()/model()/parent()/data()/isValid()/child()/sibling() | **缺** → ClegModelIndex（v2） |
| QGraphicsView | 2D 场景视图 | setScene/scene/fitInView/scale/rotate/translate/itemAt/mapToScene | **缺** → ClegGraphicsView（可选 v2） |
| QOpenGLWidget | OpenGL 画布 | setUpdateBehavior/initializeGL/paintGL/resizeGL/grabFramebuffer（Qt6 在 QtOpenGLWidgets 模块） | ClegOpenGLWidget（已有，骨架） |
| QVideoWidget | 视频表面（QtMultimedia） | setAspectRatioMode/setFullScreen/setGeometry（配合 QMediaPlayer::setVideoOutput） | ClegVideoWidget（已有，骨架） |
| QUndoView | 撤销栈视图 | setStack/stack/cleanIcon/emptyLabel/currentIndexChanged | **缺** → ClegUndoView（可选） |
| QSizeGrip | 角落拖拽把手 | sizeHint/setVisible；QSS width/height/image | ClegSizeGrip（已有） |
| QLCDNumber | 七段数码管 | display(int/double/QString)/value/setDigitCount/setMode/setSegmentStyle/setSmallDecimalPoint/setHexMode | ClegLCDNumber（已有） |
| QFontComboBox | 字体选择下拉 | currentFont/setCurrentFont/setFontFilters/writingSystem | **缺** → ClegFontComboBox |
| QDialogButtonBox | 见上 | — | 已有 |
| QMessageBox | 消息对话框 | 静态 information/warning/critical/question/about；setText/setInformativeText/setStandardButtons/exec；buttonClicked | **缺** → ClegMessageBox |
| QFileDialog | 文件对话框 | 静态 getOpenFileName/getOpenFileNames/getSaveFileName/getExistingDirectory/getExistingDirectoryUrl；setFileMode/setNameFilter/selectedFiles | **缺** → ClegFileDialog |
| QColorDialog | 颜色对话框 | 静态 getColor/getColorFromRgba；setCurrentColor/currentColor | **缺** → ClegColorDialog |
| QFontDialog | 字体对话框 | 静态 getFont；setCurrentFont/currentFont/currentFontChanged | **缺** → ClegFontDialog |
| QInputDialog | 输入对话框 | 静态 getText/getMultiLineText/getInt/getDouble/getItem；setLabelText/setTextValue | **缺** → ClegInputDialog |
| QCompleter | 补全器（QLineEdit 配套） | setModel/setCompletionMode/setCaseSensitivity/complete(); activated(QString)/highlighted | 可选 |
| QProgressDialog | 带取消进度对话框 | setRange/setValue/setLabelText/cancel() | 可选 |
| QAction | 动作对象（菜单/工具栏共用） | setText/setIcon/setShortcut/setCheckable/setEnabled/setVisible；triggered(bool)/toggled(bool)/changed/hovered | **缺**（cleg 菜单无动作模型）→ ClegAction（应有） |
| QSplashScreen / QWizard / QRubberBand / QSystemTrayIcon / QKeySequenceEdit | 其他常用 | — | 可选 |

---

## 5. 信号槽模型（表：信号 | 参数 | 触发时机）

### 5.1 声明与连接语法（来源：<https://doc.qt.io/qt-6/signalsandslots.html>）

- 声明：类须继承 QObject 并含 **`Q_OBJECT`** 宏；`signals:` 区声明信号（由 moc 生成，**不要在 .cpp 实现**）；`public slots:`/`private slots:` 区声明槽（普通成员函数）；`emit signalName(args)` 发射。
- 连接：
  - 新式（函数指针，编译期类型检查 + 自动断链）：`QObject::connect(sender, &QPushButton::clicked, receiver, &MyClass::onClicked);`
  - 旧式（字符串宏，运行期检查）：`connect(sender, SIGNAL(clicked()), this, SLOT(onClicked()));`
  - Lambda（需 context 对象）：`connect(btn, &QPushButton::clicked, this, [=]{ ... });`
  - 重载信号用 `qOverload<T>(&QCombo::activated)`；`Qt::UniqueConnection` 防重复；槽参数可比信号少（末位省略）。
- 语义要点：一个信号可连多槽（按连接顺序执行）；信号可连信号；`emit` 后槽同步执行（除 QueuedConnection）；sender/receiver 任一销毁自动断开；`QObject::sender()` 取发送者；`disconnect()` 断开。

### 5.2 常用内建信号（参数与触发时机已按官方页逐条核对）

| 信号 | 参数 | 触发时机 | 来源 |
|---|---|---|---|
| `QAbstractButton::clicked` | `bool checked=false` | 按钮被激活（按下并释放）时 | qabstractbutton |
| `QAbstractButton::pressed` / `released` | 无 | 按下瞬间 / 释放瞬间 | 同上 |
| `QAbstractButton::toggled` | `bool checked` | 勾选状态翻转 | 同上 |
| `QPushButton`（继承） | — | `:default/:flat/:checked` 伪状态对应 | qpushbutton |
| `QLineEdit::textChanged` | `QString` | 文本变化（程序或用户） | qlineedit |
| `QLineEdit::textEdited` | `QString` | 仅用户编辑时 | 同上 |
| `QLineEdit::returnPressed` | 无 | 回车 | 同上 |
| `QLineEdit::editingFinished` | 无 | 编辑完成（失焦/回车） | 同上 |
| `QLineEdit::cursorPositionChanged` | `int,int` | 光标移动 | 同上 |
| `QLineEdit::selectionChanged` | 无 | 选中变化 | 同上 |
| `QComboBox::currentIndexChanged` | `int` | 当前索引变化 | qcombobox |
| `QComboBox::currentTextChanged` | `QString` | 当前文本变化 | 同上 |
| `QComboBox::activated` | `int` | 用户选择（程序 setCurrentIndex 不触发） | 同上 |
| `QComboBox::highlighted` | `int` | 悬停高亮变化 | 同上 |
| `QComboBox::editTextChanged` | `QString` | 可编辑模式下文本变化 | 同上 |
| `QAbstractSlider::valueChanged` | `int` | 值变化（QSlider/QScrollBar/QDial 共用） | qabstractslider |
| `QAbstractSlider::sliderMoved` | `int` | 拖动中 | 同上 |
| `QAbstractSlider::sliderPressed` / `sliderReleased` | 无 | 按下/松手 | 同上 |
| `QAbstractSlider::rangeChanged` | `int,int` | 范围变化 | 同上 |
| `QAbstractSlider::actionTriggered` | `int` | 页步/行步动作触发 | 同上 |
| `QSpinBox::valueChanged` | `int` | 值变化（QDoubleSpinBox 为 double） | qspinbox |
| `QAbstractSpinBox::editingFinished` / `returnPressed` | 无 | 编辑完成/回车 | qabstractspinbox |
| `QDateTimeEdit::dateTimeChanged` / `dateChanged` / `timeChanged` | `QDateTime`/`QDate`/`QTime` | 相应字段变化 | qdatetimeedit |
| `QCalendarWidget::selectionChanged` | 无 | 选中日期变化 | qcalendarwidget |
| `QCalendarWidget::currentPageChanged` | `int year,int month` | 翻页 | 同上 |
| `QCalendarWidget::clicked` / `activated` | `QDate` | 单击/双击/回车 | 同上 |
| `QListWidget::itemClicked` | `QListWidgetItem*` | 单击项 | qlistwidget |
| `QListWidget::itemDoubleClicked` / `itemActivated` | `QListWidgetItem*` | 双击/激活（双击或回车） | 同上 |
| `QListWidget::currentRowChanged` | `int` | 当前行变化 | 同上 |
| `QListWidget::currentItemChanged` | `*,*` | 当前项变化（旧,新） | 同上 |
| `QListWidget::itemSelectionChanged` | 无 | 选择集变化 | 同上 |
| `QTreeWidget::itemClicked` / `itemDoubleClicked` / `itemActivated` | `QTreeWidgetItem*,int column` | 单击/双击/激活 | qtreewidget |
| `QTreeWidget::itemExpanded` / `itemCollapsed` | `QTreeWidgetItem*` | 展开/折叠 | 同上 |
| `QTreeWidget::itemChanged` | `QTreeWidgetItem*, int` | 项数据变化 | 同上 |
| `QTableWidget::cellClicked` / `cellDoubleClicked` / `cellActivated` | `int row,int column` | 单元格单击/双击/激活 | qtablewidget |
| `QTableWidget::cellChanged` | `int,int` | 单元格内容变化 | 同上 |
| `QTableWidget::currentCellChanged` | `int,int,int,int` | 当前格变化（旧行,旧列,新行,新列） | 同上 |
| `QTableWidget::itemClicked` | `QTableWidgetItem*` | 项单击 | 同上 |
| `QTabWidget::currentChanged` | `int index` | 当前页变化 | qtabwidget |
| `QTabWidget::tabBarClicked` / `tabBarDoubleClicked` | `int index` | 点标签栏 | 同上 |
| `QTabWidget::tabCloseRequested` | `int index` | 点关闭按钮 | 同上 |
| `QStackedWidget::currentChanged` | `int index` | 页面切换 | qstackedwidget |
| `QStackedWidget::widgetAdded` / `widgetRemoved` | `int index` | 页增删 | 同上 |
| `QMenu::triggered` | `QAction*` | 菜单项被触发 | qmenu |
| `QMenu::hovered` | `QAction*` | 悬停项变化 | 同上 |
| `QAction::triggered` | `bool checked=false` | 动作触发 | qaction |
| `QAction::toggled` | `bool checked` | 勾选动作翻转 | 同上 |
| `QDialogButtonBox::accepted` / `rejected` | 无 | 点 OK/Accept / Cancel/Reject | qdialogbuttonbox |
| `QDialogButtonBox::clicked` | `QAbstractButton*` | 任意按钮点击 | 同上 |
| `QHeaderView::sectionClicked` / `sectionDoubleClicked` / `sectionPressed` | `int logicalIndex` | 表头点击 | qheaderview |
| `QHeaderView::sectionResized` | `int,int,int,int` | 分段尺寸变化 | 同上 |
| `QHeaderView::sortIndicatorChanged` | `int,Qt::SortOrder` | 排序指示变化 | 同上 |
| `QGroupBox::toggled` | `bool` | 勾选状态翻转 | qgroupbox |
| `QGroupBox::clicked` | `bool checked=false` | 点击标题 | 同上 |
| `QProgressBar::valueChanged` | `int` | 进度值变化 | qprogressbar |
| `QWidget::customContextMenuRequested` | `const QPoint&` | 右键菜单请求（contextMenuPolicy 相关） | qwidget |
| `QWidget::windowTitleChanged` | `QString` | 标题变化 | qwidget |
| `QWidget::windowIconChanged` | `QIcon` | 图标变化 | qwidget |
| `QAbstractItemView::clicked` | `QModelIndex` | 视图通用点击（QListView/QTreeView/QTableView 继承） | qabstractitemview |

---

## 6. 布局体系（表）

### 6.1 布局类与方法（来源：<https://doc.qt.io/qt-6/qlayout.html>、<https://doc.qt.io/qt-6/qboxlayout.html>、<https://doc.qt.io/qt-6/qgridlayout.html>、<https://doc.qt.io/qt-6/qformlayout.html>、<https://doc.qt.io/qt-6/qspaceritem.html>）

| Qt 布局类 | 关键方法 | 语义 | cleg 现状/建议 |
|---|---|---|---|
| QLayout（基类） | `addWidget`、`addLayout`、`setSpacing`、`setContentsMargins(l,t,r,b)`、`setSizeConstraint(SizeConstraint)`、`activate()`、`invalidate()`、`widget()/count()/itemAt()` | 布局算法统一入口；SizeConstraint 控制超出 hint 收缩策略（SetDefaultConstraint/SetFixedSize/SetMinAndMaxSize…） | 缺；`cleglayout::vbox/hbox/grid` 仅逐个写 `pos` |
| QBoxLayout | `addWidget/addLayout/addSpacing/addStretch/addSpacerItem/setStretch/insertWidget/setDirection/setSpacing/setContentsMargins` | 一维排布；stretch 因子控弹性 | 建议 `layout="vbox|hbox"` 风格键 + `stretch` 键 |
| QGridLayout | `addWidget(w,row,col,rowSpan,colSpan,alignment)`、`setSpacing/setHorizontalSpacing/setVerticalSpacing`、`addItem`、`setColumnMinimumWidth`、`setRowStretch/setColumnStretch`、`setOriginCorner` | 二维排布，可跨行跨列 | `cleglayout::grid` 无跨列/行高系数 |
| QFormLayout | `addRow(label, field)`、`addRow-1`、`setSpacing`、`setLabelAlignment`、`setFieldGrowthPolicy`、`setRowWrapPolicy`、`addItem` | 标签-字段行对 | 缺，建议 `ClegFormLayout` 方法集 |
| QSpacerItem | `width()/height()/sizeHint()/expandingDirections()`、`QBoxLayout::addStretch(n)` | 弹性区间 | 缺 → 建议 `stretch` 键（等价 addStretch） |
| QWidget 集成 | `setLayout()`、`layout()`、`QWidget::sizeHint()`、`updateGeometry()`（尺寸元数据变化后通知布局重算）、`hasHeightForWidth()/heightForWidth(w)` | widget 侧支持 | 缺（无布局通知通道） |
| 顶层窗口约束 | QWidget 文档：顶层窗口初始尺寸限制为桌面 2/3（可 resize() 突破） | — | 记录 |

### 6.2 sizePolicy 与 sizeHint 语义（来源：<https://doc.qt.io/qt-6/qsizepolicy.html>、<https://doc.qt.io/qt-6/qwidget.html#Size-Hints-and-Size-Policies>）

| QSizePolicy::Policy | 含义（官方原文要点） | 典型控件 |
|---|---|---|
| `Fixed` (0) | 只能等于 sizeHint，不可大不可小 | 按钮竖向 |
| `Minimum` | sizeHint 为下限，可长大无益 | 按钮横向 |
| `Maximum` | sizeHint 为上限，可任意缩小 | 分隔线 |
| `Preferred`（默认） | sizeHint"最佳"，可大可小 | 多数默认 |
| `Expanding` | 可大（占剩余空间）可小 | QLineEdit/QListView |
| `MinimumExpanding` | 下限即 minSizeHint，倾向扩展 | 视图类 |
| `Ignored` | sizeHint 被忽略，任意分配 | 自缩放手势控件 |

- `sizeHint()`：理想推荐尺寸；`minimumSizeHint()`：内容不劣化最小尺寸；`QSize` 二维。实现新控件 = 重写 `sizeHint` + `setSizePolicy`（QWidget 官方建议）。
- `setMinimumSize/setMaximumSize/setFixedSize` 为 widget 级硬约束，布局尊重约束并取 `clamp(sizeHint)`。

---

## 7. cleg 差距清单（重点）

**cleg 现状（实读 `cleg.qk` / `clegfb.go`，非推测）**：
- `ClegNode` 接口（dynamic）：`render / setStyle(jsonText) / getStyle / getX / getY / getW / getH`。
- `style HashTable<String,String>`：11 个旧键 `text/items/tabs/rows/menus/size/pos/font/color/bg/title`；QSS 名键仅 `color/background-color/font-size/font-family/border-radius` 在 render 中经 `qss::st/num/cr` 读取（部分组件）。
- 47 个组件 struct + impl ClegNode；状态仅 ClegCheckBox/ClegRadioButton 有 `checked bool` 字段；无 hover/pressed/disabled/focus 渲染分支。
- 布局：`cleglayout::{vbox,hbox,grid}` 三个函数 = 直接改写 `style["pos"]`；无 margins/spacing 样式键（spacing 是函数参数）。
- 信号：`clegsignal::emit(node, name)` → `qksignal_emit` 触发节点方法（如 `onClicked`）；无参数、无 connect/disconnect、无多播。
- 渲染：CPU 帧缓冲（ARGB u32）+ 5x7 位图字体；文本回退链 `drawTextChain`：按 `font` 链逐个探测 TTF（freetype）→ 全失败回退 5x7 位图。无 png/背景图/九宫格、无文本度量（宽度/居中）、无裁剪边界（wrap/middle）。

### 7.1 必有（对齐 Qt 基本使用模式，件件阻断）

| # | 缺口 | Qt 参照 | 可执行建议（style 键或方法名） |
|---|---|---|---|
| C1 | **无 QSS 键全集**：仅 5 个 QSS 键 + 11 个旧键 | margin/padding/border-*/background-*/color/font-*/text-align/text-decoration（第 2 节全表） | 逐组件把 render 的取色/取文统一改为 `qss::num/cr/st` 读取以下键：`padding`、`margin`、`border-width`、`border-style`、`border-color`、`text-align`、`text-decoration`、`font-style`、`font-weight`、`letter-spacing` |
| C2 | **无盒模型**：无 content 矩形三区（margin/border/padding），`border-radius` 已直接在 (x,y,w,h) 上画 | QSS box model（background-clip/origin） | 新增 `getContentRect()` 派生：content = (x+margin+border+padding…)；render 统一先算 content rect 再画；键 `margin`/`padding`/`border-width` 解析为四元组 `"1,2,1,2"` 或 `"2"` |
| C3 | **无伪状态**：hover/pressed/disabled/checked(仅字段)/focus/selected 无 style 表达 | 45 个伪状态 | ① 事件层维护节点状态位；② 键命名约定 `"<qss名>:<state>"`，e.g. `style["background-color:hover"]="240,98,146"`；③ 扩展 `qss::cr(state,...)` 辅助函数带 state 参数：`cr(style, "background-color", state, 0, fb)`；④ 含 `checked` 的组件（现为 bool 字段）统一并入状态位 |
| C4 | **无事件/命中测试**：没有 mouse press/move/hover/键盘分发，`onClicked` 触发源不明 | QMouseEvent/QKeyEvent + paintEvent 等事件虚函数 | `cleg::dispatchEvent(x,y,kind)`：命中测试采用 `hitTest(node,x,y)`（按 getX/getY/getW/getH 倒序 z）；节点协议新增 `dynamic fn onMouseDown(self, x,y)`/`onMouseUp`/`onHover(enter bool)`/`onKey`，运行库转发到 `clegsignal::emit(node,"clicked")` |
| C5 | **布局不计算尺寸**：只有"写 pos"函数，无 minimum/maximum/preferred 元数据、无 sizeHint、无 stretch | QSizePolicy 六策略 + sizeHint/minimumSizeHint | ① ClegNode 增补只读方法：`getMinW/getMinH/getMaxW/getMaxH`（默认从 style 键 `min-width/min-height/max-width/max-height` 读，未设回退 0/QWIDGETSIZE_MAX）；② 键 `size-policy`（`"h,v"` 两轴，值 fixed/minimum/maximum/preferred/expanding/ignored）；③ ClegLayout 遍历：先收集 sizeHint（键 `size-hint="w,h"` 或派生 `size` 旧键），再按策略分配 |
| C6 | **stretch/弹性缺席**：无 QSpacerItem、无 stretch 因子 | QBoxLayout::addStretch/setStretch | 键 `stretch="1"`（默认 0）；ClegLayout::vbox 累计 stretch 比例分配剩余空间 |
| C7 | **信号无参无订阅**：`emit(node,name)` 无法携带 value/text/index，也无槽注册（仅约定方法名） | 带参信号 + connect(sender,signal,receiver,slot) | ① `clegsignal::on(node, "valueChanged", handler)` 注册表（节点上动态字段或 HashTable<String, fn>）；② `emit(node, "valueChanged", value)` 支持 int/string 参数；③ 与 Qt 对齐的语义：多播顺序、disconnect、销毁自动断开 |
| C8 | **无 enabled/disabled/focus 状态与语义**：没有 setEnabled()/setFocus()/focusPolicy | QWidget::enabled/focusPolicy/focus | 键 `enabled="false"` / `focus-policy="strong"`；运行库维护**焦点链**（下一个/上一个，Tab 键），`:focus` 派生 `state` |
| C9 | **文本不含样式与对齐**：无 text-align/text-decoration/font-style/weight；文本以左上角硬画 | text-align/text-decoration/font | `text-align="left|center|right"` + `text-valign="top|middle|bottom"`；需要**文本度量**：`qkcleg_text_width(text,size,font)` 才能居中/右对齐（TTF 用 metrics，5x7 用 6*scale 近似） |
| C10 | **无对象名/选择器匹配**：所有组件同样式；无 `#objectName` 定向 | ID 选择器 `QPushButton#okButton`、属性选择器 | ① ClegNode 增 `objectName` 字段（HashTable 亦可 `style["object-name"]`）；② `setStyle(json)` 升级为支持 `{"QPushButton#ok": {...}, "QPushButton:hover": {...}}` 选择器块键值；③ 布局/事件遍历时按类型+objectName+状态三重匹配合并样式（级联：祖先→自身覆盖，对应 Qt cascading） |
| C11 | **背景只有纯色**：无 background-image/重复/九宫格 | background-image/background-repeat/border-image | v2 起：`background-image="path"`、`background-repeat="repeat|no-repeat|repeat-x|repeat-y"`、`border-image="path"`；CPU 后端补纹理光栅（u32 拷入+alpha 混合） |
| C12 | **无字号单位与度量**：`font-size` 仅当"位图缩放系数"，pt 不识 | QSS font-size 仅 pt/px | `font-size="10pt"` 按 DPI 折 px（96dpi 基准）；px 直用；`scaleFor` 修正为真实字号→行高 |
| C13 | **字体链健壮性**：`fontPathOf` 逐名探测（空格/别名敏感），无 weight/style 映射 | QFont 族+weight/style 系统字体库 | 抽样标准化：小写、去空格、别名表（"monospace"→若干等宽族）；`font-weight`/`font-style` 传给 ftLoadFace（bold/italic 变体选择）；链末 5x7 兜底已具备（保留） |

### 7.2 应有（主流组件与常见用法补全）

| # | 缺口 | Qt 参照 | 可执行建议 |
|---|---|---|---|
| D1 | 缺 `ClegDateTimeEdit`（有 Date/Time） | QDateTimeEdit | 新增 struct；键 `date`/`time`/`datetime`/`display-format`；信号 `dateTimeChanged` |
| D2 | 缺 `ClegToolButton`/`ClegCommandLinkButton`；ClegButton 仅 QPushButton 语义 | QToolButton/QCommandLinkButton | `ClegToolButton`（键 `icon`/`tool-button-style`，信号 `triggered`）；`ClegCommandLinkButton`（键 `description`） |
| D3 | 缺对话框五件套 | QMessageBox/QFileDialog/QColorDialog/QFontDialog/QInputDialog | `ClegMessageBox`（静态 `information/warning/error/question` + 键 `standard-buttons`）+ `ClegFileDialog`（静态 `getOpenFileName`）+ `ClegColorDialog`（`getColor`）+ `ClegFontDialog`（`getFont`）+ `ClegInputDialog`（`getText/getInt/getItem`）；均加 `modal` 语义 |
| D4 | 缺 `ClegFontComboBox` | QFontComboBox | 复用字体回退链枚举族名；键 `font-filters` |
| D5 | 缺 `ClegButtonGroup`（互斥） | QButtonGroup | `addButton(node, id)`/`checkedId()`/`idClicked(id)`；RadioButton 互斥需它 |
| D6 | 缺动作模型 | QAction（menu/toolbar 共用） | `ClegAction`（text/icon/shortcut/checkable/enabled）；ClegMenu/ClegToolBar 用 `addAction(node)`；信号 `triggered(bool)` |
| D7 | TabWidget/StackedWidget 子页缺增删 API | addTab/removeTab/currentChanged(int) | `ClegTabWidget::addTab(node,label)`、`setCurrentIndex`；键 `tab-pos`（top/bottom/left/right） |
| D8 | 进度/滑条子件无样式 | `::chunk`/`::groove`/`::handle`/`tick` | 键 `chunk-bg`/`chunk-height`、`groove-bg`、`handle-bg`/`handle-size`、`tick-position` |
| D9 | 表/列表选中与网格线无样式 | selection-background-color/selection-color/gridline-color/alternate-background-color | 键 `selection-bg`/`selection-color`/`gridline-color`/`alternate-bg`；列表项悬停 `item:hover`（复用 C3 状态后缀约定） |
| D10 | 表头子件无样式 | `::section`（:first/:last/:selected 等） | 键 `section-bg`/`section-bg:hover`/`section-color`；排序箭头 `sort-arrow` |
| D11 | 菜单/工具栏项无状态样式 | `::item`（:selected/:default）、`::separator`、`::indicator` | 键 `menu-item-bg`/`menu-item-bg:selected`/`menu-separator-color`/`indicator-bg:checked` |
| D12 | 输入框无占位符/回显模式 | placeholder-text-color/echo-mode（password） | 键 `placeholder`/`placeholder-text-color`/`echo-mode="normal|password"`/`password-char` |
| D13 | 主窗口无布局槽位 | menuBar/statusBar/centralWidget/addDockWidget/addToolBar | `ClegMainWindow` 增方法 `setCentralWidget(node)/addDockWidget(node)/addToolBar(node)` + 键 `menu-bar`/`status-bar`（节点引用存 HashTable 需存 id，v2 或引用表） |
| D14 | 无 palette 角色体系 | QPalette（Window/Button/Text/Highlight/Active/Disabled） | 键 `palette.window`/`palette.button`/`palette.text`/`palette.highlight`/`palette.disabled.text`；组件缺省键时沿调色板再回退 |
| D15 | 无 cursor 与 tooltip 渲染 | cursor/toolTip | 键 `cursor="hand|text|arrow|move|cross"`（宿主层实现）；`tooltip` 已留键，需事件层 `onHover` 后画 tip 气泡（圆角+`tooltip` 主题） |
| D16 | 无 QToolTip 全局主题 | QToolTip（box model 全支持） | `cleg::setGlobalStyle(key,val)` → 全局样式表 HashTable，组件样式合并（对应 QApplication::setStyleSheet） |
| D17 | 无模型/委托概念 | QItemDelegate/QModelIndex | v2：`ClegItemDelegate`（`onPaint/onEdit`）；`ClegModelIndex(row,col)` 值类型；ClegListView/ClegTableView 委托回调 |
| D18 | 无窗框/窗口装饰策略 | windowFlags/windowModality | 键 `window-flags="frameless|tool|dialog"`、`window-modality="application|window"` |
| D19 | 无 F1/whatsThis/状态提示 | whatsThis/statusTip | 键 `whats-this`/`status-tip`（statusbar 显示） |

### 7.3 可选（锦上添花/后续路线）

| # | 缺口 | cleg 建议 |
|---|---|---|
| E1 | 真 CSS 解析器（`.qss` 文件：选择器+声明+伪态+子控件+优先级） | JSON 式 `setStyle` 升级为 `cleg::loadStylesheet("theme.qss")`；内部编译为 (selector,key,value) 列表；specificity 按 CSS2 规则排序 |
| E2 | QGraphicsView/QGraphicsScene 2D 场景树 | `ClegGraphicsView`：场景节点树 + Z 序 + 拾取（事件层可复用） |
| E3 | QUndoView/撤销栈 | `ClegUndoView` + `ClegUndoStack`（push/undo/redo 回调） |
| E4 | QVideoWidget/QOpenGLWidget 实后端 | 挂 Vulkan 交换链（QuarkLangLibs-Vulkan）后做实 |
| E5 | 动画/透明度（opacity、widget-animation-duration） | `opacity` 键（u32 alpha 混合已有基础） |
| E6 | 拖放（acceptDrops/DnD）、触摸 | 键盘/鼠标事件层之后接 |
| E7 | 无障碍（accessibleName/Description）、输入法（inputMethodHints/locale）、RTL（layout-direction） | 键就位后 v3 |
| E8 | 高 DPI/设备像素比 | 字号/几何换算层 |
| E9 | 样式热替换（setStyleSheet 即时生效 = Qt 语义） | style 写入后自动标记 dirty → 下一帧 render（当前需手动重画） |
| E10 | QMdiArea 子窗口拖拽/标题（dock 样式 `::title/::close-button`） | ClegMdiSubWindow 标题、关闭按钮组件化 |

**Closure 注意点（对齐 Qt 的"驯兽指南"）**：
1. QPushButton 只设 `background-color` 不生效的坑 → 提醒用户同时设 `border`（cleg 若实现 `border-width:0` 也要给默认边框） 。
2. QLabel **不支持 `:hover`**（官方明确）→ ClegLabel 不要做 hover 分支，避免非对齐。
3. QSlider **没有** `::sub-page`、QScrollBar **有** → 键命名跟随 Qt，勿混。
4. QSS `font/color` 默认**不继承**（除非 AA_UseStyleSheetPropagationInWidgetStyles）→ cleg 全局样式默认也不继承、或提供 `cl.window > *` 等价选择器。
5. QSS 无 `!important`；同特异性"后出现优先" → cleg 合并顺序：全局 < 父 < 自身（自身永远赢）。
6. QDialog 只支持 background 三个属性；QStatusBar 只支持 background —— cleg 不必为它们扩键。

---

## 来源 URL 汇总

- QWidget: <https://doc.qt.io/qt-6/qwidget.html>（属性表/信号/事件/Size Hints & Size Policies）
- Qt Style Sheets 总览: <https://doc.qt.io/qt-6/stylesheet.html>
- QSS 参考（属性全集/可样式控件/伪状态/子控件）: <https://doc.qt.io/qt-6/stylesheet-reference.html>
- QSS 语法（选择器/级联/特异性/继承/qproperty）: <https://doc.qt.io/qt-6/stylesheet-syntax.html>
- QSS 自定义（box model）: <https://doc.qt.io/qt-6/stylesheet-customizing.html>
- QSS 示例: <https://doc.qt.io/qt-6/stylesheet-examples.html>
- 信号槽: <https://doc.qt.io/qt-6/signalsandslots.html>
- Qt Widgets 模块类清单: <https://doc.qt.io/qt-6/qtwidgets-module.html>
- QSizePolicy: <https://doc.qt.io/qt-6/qsizepolicy.html>
- 布局: <https://doc.qt.io/qt-6/qlayout.html>、<https://doc.qt.io/qt-6/qboxlayout.html>、<https://doc.qt.io/qt-6/qgridlayout.html>、<https://doc.qt.io/qt-6/qformlayout.html>、<https://doc.qt.io/qt-6/qspaceritem.html>、<https://doc.qt.io/qt-6/qlayoutitem.html>
- 各控件信号/属性核对页：qabstractbutton / qlineedit / qcombobox / qlistwidget / qtreewidget / qtablewidget / qabstractslider / qabstractspinbox / qspinbox / qdatetimeedit / qcalendarwidget / qtabwidget / qstackedwidget / qmenu / qaction / qheaderview / qgroupbox / qprogressbar / qtoolbutton / qwidget（均在 <https://doc.qt.io/qt-6/> 下，如 <https://doc.qt.io/qt-6/qlineedit.html>）
- 跨模块类：QOpenGLWidget → QtOpenGLWidgets 模块 <https://doc.qt.io/qt-6/qtopenglwidgets-module.html>；QVideoWidget → QtMultimedia <https://doc.qt.io/qt-6/qvideowidget.html>

> 本报告为查证结果；第 7 节 cleg 差距来自对 `QuarkLangLibs-Cleg/cleg.qk`（47 组件、ClegNode 接口、cleglayout/clegsignal/qss 空间）与 `QuarkLang/internal/lang/clegfb.go`（CPU 帧缓冲 + drawTextChain 字体回退链）的实读代码。
