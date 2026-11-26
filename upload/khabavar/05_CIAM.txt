KHABAVAR — Central Identity & Access Management
معماری هویت، احراز هویت و دسترسی یکپارچه
Document ID: KH-IAM-001
Version: 1.0
Status: Architecture Baseline
Architecture: Central Identity / Modular Monolith
1. تصمیم معماری
تمام سرویس‌های خواب‌آور از یک سیستم هویت مرکزی استفاده می‌کنند.
موجودیت Customer فقط یک‌بار در سیستم ایجاد می‌شود و شناسه آن در تمام سرویس‌ها ثابت خواهد بود.
سرویس‌های وابسته:
* www.khabavar.ir — وب‌سایت اصلی
* shop.khabavar.ir — فروشگاه
* magazine.khabavar.ir — مجله و محتوای آموزشی
* سایر زیردامنه‌های آینده
هر سرویس می‌تواند اطلاعات اختصاصی خود را داشته باشد، اما مالکیت هویت و احراز هویت در ماژول مرکزی قرار می‌گیرد.
2. ساختار منطقی
                 KHABAVAR IDENTITY
                        |
             Central Identity Module
                        |
              Authentication Core
                        |
              Authorization Engine
                        |
          +-------------+-------------+
          |             |             |
        SHOP         MAGAZINE       WEBSITE
          |             |             |
       Commerce       Content       General
       Customer       Reader        Profile
       Admin          Editor        Settings
3. معماری ورود یکپارچه
پیشنهاد می‌شود جریان ورود بر اساس OpenID Connect و Authorization Code Flow با PKCE طراحی شود.
دامنه احراز هویت:
auth.khabavar.ir
جریان ورود:
1. کاربر وارد فروشگاه می‌شود.
2. فروشگاه بررسی می‌کند که نشست معتبر وجود دارد یا خیر.
3. در صورت نبود نشست، کاربر به سامانه احراز هویت مرکزی هدایت می‌شود.
4. کاربر با شماره تلفن و OTP احراز هویت می‌شود.
5. سامانه هویت مرکزی اعتبار کاربر را تأیید می‌کند.
6. کاربر با مجوز و کد یک‌بارمصرف به سرویس درخواست‌کننده بازمی‌گردد.
7. سرویس نشست اختصاصی خود را ایجاد می‌کند.
کد ورود باید کوتاه‌عمر، یک‌بارمصرف و محدود به درخواست احراز هویت مشخص باشد.
4. روش احراز هویت
روش اصلی نسخه اول
ورود با شماره تلفن همراه و OTP.
ویژگی‌ها:
* شماره تلفن به‌عنوان شناسه ورود.
* تأیید مالکیت شماره تلفن.
* محدودیت دفعات درخواست OTP.
* محدودیت دفعات ورود اشتباه.
* انقضای خودکار OTP.
* جلوگیری از استفاده مجدد از کد.
* ثبت رویدادهای امنیتی.
اتصال به سرویس پیامکی واقعی تا زمان انتخاب و تأیید تأمین‌کننده پیامک فعال نمی‌شود.
روش‌های توسعه آینده
* ورود با ایمیل
* ورود با حساب‌های خارجی
* Passkeys
* احراز هویت دومرحله‌ای برای مدیران
5. مدیریت نشست و امنیت مرورگر
* نشست‌ها باید در سمت سرور قابل ابطال باشند.
* Cookieها باید HttpOnly و Secure باشند.
* SameSite متناسب با جریان احراز هویت تنظیم شود.
* نشست هر سرویس از نشست مرکزی مستقل باشد.
* توکن‌های حساس در LocalStorage قرار نگیرند.
* خروج از حساب باید امکان ابطال نشست‌های فعال را فراهم کند.
* بازگشت از سامانه هویت فقط به Redirect URI ثبت‌شده و مجاز انجام شود.
اشتراک Cookie میان تمام زیردامنه‌ها به‌صورت پیش‌فرض انجام نمی‌شود.
6. مدل مجوزها
مدل دسترسی مبتنی بر RBAC همراه با Scopeهای سرویس‌محور خواهد بود.
نقش‌های عمومی
* CUSTOMER
* ADMIN
* SUPER_ADMIN
* SUPPORT
* CONTENT_EDITOR
* FINANCE
* INVENTORY_MANAGER
* SUPPLIER_OPERATOR
نمونه مجوزها
سرویس
	مجوز
	Shop
	shop:orders:read
	Shop
	shop:orders:create
	Shop
	shop:returns:create
	Shop
	shop:inventory:manage
	Magazine
	magazine:articles:read
	Magazine
	magazine:articles:edit
	Admin
	admin:users:manage
	Finance
	finance:refunds:approve
	نقش به‌تنهایی برای دسترسی به عملیات حساس کافی نیست؛ مجوز مشخص عملیات نیز باید بررسی شود.
7. ساختار دیتابیس IAM
users
* id UUID
* phone VARCHAR
* email VARCHAR
* display_name VARCHAR
* status TEXT
* phone_verified_at TIMESTAMPTZ
* email_verified_at TIMESTAMPTZ
* created_at TIMESTAMPTZ
* updated_at TIMESTAMPTZ
user_credentials
* id UUID
* user_id UUID
* credential_type TEXT
* credential_reference TEXT
* password_hash TEXT
* created_at TIMESTAMPTZ
* updated_at TIMESTAMPTZ
user_sessions
* id UUID
* user_id UUID
* session_token_hash TEXT
* client_id UUID
* ip_address INET
* user_agent TEXT
* expires_at TIMESTAMPTZ
* revoked_at TIMESTAMPTZ
* created_at TIMESTAMPTZ
identity_clients
* id UUID
* client_id VARCHAR
* client_name VARCHAR
* client_type TEXT
* redirect_uris JSONB
* allowed_scopes JSONB
* status TEXT
roles
* id UUID
* code VARCHAR
* name VARCHAR
* description TEXT
permissions
* id UUID
* code VARCHAR
* description TEXT
user_roles
* user_id UUID
* role_id UUID
* scope_type TEXT
* scope_id UUID
role_permissions
* role_id UUID
* permission_id UUID
otp_challenges
* id UUID
* phone VARCHAR
* code_hash TEXT
* purpose TEXT
* attempts INTEGER
* expires_at TIMESTAMPTZ
* consumed_at TIMESTAMPTZ
* created_at TIMESTAMPTZ
OTP باید به‌صورت Hash امن ذخیره شود و پس از مصرف یا انقضا دیگر قابل استفاده نباشد.
8. تفکیک پروفایل مشتری
اطلاعات هویتی در Identity نگهداری می‌شود.
اطلاعات تجاری مشتری در Commerce:
customer_profiles
* user_id UUID
* customer_type TEXT
* marketing_consent BOOLEAN
* created_at TIMESTAMPTZ
* updated_at TIMESTAMPTZ
customer_addresses
* id UUID
* customer_id UUID
* province TEXT
* city TEXT
* postal_code VARCHAR
* address_line TEXT
* recipient_name TEXT
* recipient_phone VARCHAR
* is_default BOOLEAN
توجه: پروفایل فروشگاهی نباید با پروفایل عمومی کاربر در اکوسیستم یکی فرض شود.
9. حریم خصوصی و مدیریت اطلاعات
* جمع‌آوری حداقل اطلاعات لازم.
* رضایت جداگانه برای پیام‌های بازاریابی.
* امکان مشاهده و اصلاح اطلاعات حساب.
* امکان درخواست حذف حساب مطابق الزامات قانونی و تعهدات نگهداری سوابق مالی.
* ثبت تاریخچه رویدادهای امنیتی.
* محدودیت دسترسی کارکنان به اطلاعات شخصی.
* عدم نمایش اطلاعات مشتری به تأمین‌کنندگان غیرمرتبط.
10. معیارهای پذیرش
* یک کاربر فقط یک هویت مرکزی داشته باشد.
* ورود در چند سرویس با یک حساب ممکن باشد.
* هر سرویس نشست مستقل داشته باشد.
* مجوزهای سرویس‌ها قابل تفکیک باشند.
* OTP یک‌بارمصرف باشد.
* نشست قابل ابطال باشد.
* دسترسی غیرمجاز به اطلاعات سفارش سایر مشتریان مسدود شود.
* ثبت‌نام مجدد با شماره تأییدشده تکراری ایجاد نشود.
* احراز هویت مرکزی بدون وابستگی به ارائه‌دهنده پیامک خاص طراحی شود.
11. توسعه آینده
معماری باید قابلیت تبدیل ماژول هویت به سرویس مستقل را داشته باشد، اما این جداسازی تا زمانی که نیاز عملیاتی واقعی ایجاد نشده انجام نمی‌شود.
اصل نهایی: یک هویت در اکوسیستم، دسترسی‌های مستقل در سرویس‌ها و یک تجربه ورود پیوسته برای مشتری.